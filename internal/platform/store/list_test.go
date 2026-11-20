package store

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"
)

var itemSpec = ListSpec{
	Sortable:   map[string]string{"title": "title", "created_at": "n"},
	Searchable: []string{"title", "note"},
	Default:    "-created_at",
	Tie:        "id",
}

func parse(t *testing.T, query string) (ListParams, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/things?"+query, nil)
	return ParseList(r, itemSpec)
}

func TestAListAskedWithNoParametersIsNotPaged(t *testing.T) {
	p, err := parse(t, "")
	if err != nil || p.Paged {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = parse(t, "other=1")
	if err != nil || p.Paged {
		t.Fatalf("a filter of the list is not a list parameter: %+v %v", p, err)
	}
}

func TestLimitAndOffsetAreChecked(t *testing.T) {
	for _, bad := range []string{"limit=0", "limit=abc", "limit=501", "limit=-3", "offset=-1", "offset=x"} {
		_, err := parse(t, bad)
		var le *ListError
		if !errors.As(err, &le) {
			t.Errorf("%s: err = %v", bad, err)
			continue
		}
		name := strings.Split(bad, "=")[0]
		if !strings.Contains(le.Message, name) {
			t.Errorf("%s: the message must name the parameter: %q", bad, le.Message)
		}
	}
	if p, err := parse(t, "limit=500"); err != nil || p.Limit != 500 {
		t.Errorf("limit 500 is allowed: %+v %v", p, err)
	}
	p, err := parse(t, "offset=30")
	if err != nil || p.Limit != DefaultLimit || p.Offset != 30 {
		t.Errorf("an offset alone takes the default limit: %+v %v", p, err)
	}
	p, _ = parse(t, "sort=title")
	if !p.Paged || p.Limit != 0 {
		t.Errorf("only sort: paged with no limit: %+v", p)
	}
}

func TestSortNamesAFieldThatIsNotSortable(t *testing.T) {
	_, err := parse(t, "sort=password")
	if err == nil || !strings.Contains(err.Error(), "created_at") || !strings.Contains(err.Error(), "title") {
		t.Fatalf("the error must name the fields that can be used: %v", err)
	}
	p, err := parse(t, "sort=-title&q=%20neon%20")
	if err != nil || p.Sort != "title" || !p.Desc || p.Q != "neon" {
		t.Fatalf("%+v %v", p, err)
	}
}

type thing struct {
	ID    string `gorm:"primaryKey;size:64"`
	Title string
	Note  string
	N     int
}

func seedThings(t *testing.T, n int) *Repo[thing] {
	t.Helper()
	s := newMem(t)
	if err := s.DB.AutoMigrate(&thing{}); err != nil {
		t.Fatal(err)
	}
	r := NewRepo[thing](s, "id")
	for i := 1; i <= n; i++ {
		title := fmt.Sprintf("Thing %03d", i)
		note := ""
		if i%10 == 0 {
			title = fmt.Sprintf("NEON thing %03d", i)
		}
		if i == 7 {
			note = "100%_sure"
		}
		if err := r.Create(&thing{ID: fmt.Sprintf("t%03d", i), Title: title, Note: note, N: i}); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func TestAPageOfTwoHundredFiftyComesWithItsTotalAndItsLinks(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		r := seedThings(t, 250)
		req := httptest.NewRequest(http.MethodGet, "/api/things?limit=100&offset=100", nil)
		p, err := ParseList(req, itemSpec)
		if err != nil {
			t.Fatal(err)
		}
		rows, total, err := r.List(p, itemSpec)
		if err != nil {
			t.Fatal(err)
		}
		if total != 250 || len(rows) != 100 || rows[0].N != 150 || rows[99].N != 51 {
			t.Fatalf("total %d, %d rows, first n=%d last n=%d (newest first by default)", total, len(rows), rows[0].N, rows[len(rows)-1].N)
		}
		w := httptest.NewRecorder()
		WriteListHeaders(w, req, total, p)
		if w.Header().Get("X-Total-Count") != "250" {
			t.Errorf("X-Total-Count = %q", w.Header().Get("X-Total-Count"))
		}
		link := w.Header().Get("Link")
		if !strings.Contains(link, `offset=200`) || !strings.Contains(link, `rel="next"`) || !strings.Contains(link, `offset=0`) || !strings.Contains(link, `rel="prev"`) {
			t.Errorf("Link = %q", link)
		}
	})
}

func TestTheLastPageHasNoNextAndTheFirstHasNoPrevious(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		r := seedThings(t, 30)
		last := httptest.NewRequest(http.MethodGet, "/x?limit=20&offset=20", nil)
		p, _ := ParseList(last, itemSpec)
		_, total, _ := r.List(p, itemSpec)
		w := httptest.NewRecorder()
		WriteListHeaders(w, last, total, p)
		if l := w.Header().Get("Link"); strings.Contains(l, "next") || !strings.Contains(l, "prev") {
			t.Errorf("last page: %q", l)
		}
		first := httptest.NewRequest(http.MethodGet, "/x?limit=20", nil)
		p, _ = ParseList(first, itemSpec)
		w = httptest.NewRecorder()
		WriteListHeaders(w, first, total, p)
		if l := w.Header().Get("Link"); !strings.Contains(l, "next") || strings.Contains(l, "prev") {
			t.Errorf("first page: %q", l)
		}
		w = httptest.NewRecorder()
		WriteListHeaders(w, httptest.NewRequest(http.MethodGet, "/x", nil), total, ListParams{})
		if len(w.Header()) != 0 {
			t.Errorf("a list that was not paged gets no headers: %v", w.Header())
		}
	})
}

func TestAnOffsetBeyondTheEndIsAnEmptyListWithTheTotal(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		r := seedThings(t, 5)
		p, _ := ParseList(httptest.NewRequest(http.MethodGet, "/x?offset=50", nil), itemSpec)
		rows, total, err := r.List(p, itemSpec)
		if err != nil || len(rows) != 0 || total != 5 {
			t.Fatalf("%d rows total %d err %v", len(rows), total, err)
		}
	})
}

func TestSortAndSearchIgnoreCaseAndTreatPercentAndUnderscoreAsText(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		r := seedThings(t, 30)
		p, _ := ParseList(httptest.NewRequest(http.MethodGet, "/x?q=neon&sort=title", nil), itemSpec)
		rows, total, err := r.List(p, itemSpec)
		if err != nil || total != 3 || len(rows) != 3 || rows[0].N != 10 {
			t.Fatalf("%v total %d rows %v", err, total, rows)
		}
		p, _ = ParseList(httptest.NewRequest(http.MethodGet, "/x?q=100%25_sure", nil), itemSpec)
		rows, total, _ = r.List(p, itemSpec)
		if total != 1 || rows[0].N != 7 {
			t.Fatalf("a %% and a _ in q are text: total %d", total)
		}
		p, _ = ParseList(httptest.NewRequest(http.MethodGet, "/x?q=%25", nil), itemSpec)
		if _, total, _ = r.List(p, itemSpec); total != 1 {
			t.Fatalf("q=%% matches the one row that holds a %%: %d", total)
		}
	})
}

func TestListScopesCombineWithTheSearchAndThePage(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		r := seedThings(t, 100)
		p, _ := ParseList(httptest.NewRequest(http.MethodGet, "/x?q=thing&limit=5", nil), itemSpec)
		rows, total, err := r.List(p, itemSpec, func(db *gorm.DB) *gorm.DB { return db.Where("n > ?", 90) })
		if err != nil || total != 10 || len(rows) != 5 {
			t.Fatalf("total %d rows %d %v", total, len(rows), err)
		}
	})
}

func TestTheDefaultOrderAndTheTieMakeThePagesTotal(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		r := seedThings(t, 40)
		seen := map[string]bool{}
		for off := 0; off < 40; off += 10 {
			p, _ := ParseList(httptest.NewRequest(http.MethodGet, fmt.Sprintf("/x?limit=10&offset=%d&sort=title", off), nil), itemSpec)
			rows, _, _ := r.List(p, itemSpec)
			for _, x := range rows {
				if seen[x.ID] {
					t.Fatalf("%s is on two pages", x.ID)
				}
				seen[x.ID] = true
			}
		}
		if len(seen) != 40 {
			t.Fatalf("the pages hold %d rows of 40", len(seen))
		}
	})
}

func TestSliceTakesThePageOfAListMadeInMemory(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	for _, c := range []struct {
		p    ListParams
		want string
	}{
		{ListParams{}, "[1 2 3 4 5]"},
		{ListParams{Limit: 2}, "[1 2]"},
		{ListParams{Limit: 2, Offset: 2}, "[3 4]"},
		{ListParams{Limit: 10, Offset: 4}, "[5]"},
		{ListParams{Limit: 2, Offset: 9}, "[]"},
	} {
		if got := fmt.Sprint(Slice(items, c.p)); got != c.want {
			t.Errorf("%+v: %s, want %s", c.p, got, c.want)
		}
	}
}
