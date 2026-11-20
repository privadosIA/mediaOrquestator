package store

// list.go: the parameters every list of the API accepts (spec api-listing): limit, offset, sort and q. A handler declares what its
// list can be sorted and searched by with a ListSpec, reads the parameters with ParseList, runs the query with Repo.List (or Apply)
// and writes the headers with WriteListHeaders. A list asked with none of the four answers as it did before they existed.

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// The limits of a page.
const (
	MaxLimit     = 500
	DefaultLimit = 100 // the limit an offset with no limit gets
)

// ListSpec says what one list can be sorted and searched by.
type ListSpec struct {
	// Sortable maps the name a client sorts by to the column.
	Sortable map[string]string
	// Searchable are the text columns q is searched in, without regard to case.
	Searchable []string
	// Default is the sort when none is asked for: a name of Sortable, with a - before it for descending order. Empty leaves the
	// order of the query.
	Default string
	// Tie is a column that makes the order total (usually the primary key), so that two pages never repeat or skip a row.
	Tie string
}

// ListParams are the parameters of a list, read.
type ListParams struct {
	// Paged is false when the client sent none of limit, offset, sort and q: the handler then answers exactly as before.
	Paged  bool
	Limit  int // 0: no limit (only sort or q were sent)
	Offset int
	Sort   string // the name asked for, without the -; empty for the default
	Desc   bool
	Q      string
}

// ListError is a parameter that is not valid; the handler answers it as 400.
type ListError struct{ Message string }

func (e *ListError) Error() string { return e.Message }

// ParseList reads limit, offset, sort and q from the request, with the rules of the spec api-listing.
func ParseList(r *http.Request, spec ListSpec) (ListParams, error) {
	q := r.URL.Query()
	var p ListParams
	has := func(k string) bool { _, ok := q[k]; return ok }
	p.Paged = has("limit") || has("offset") || has("sort") || has("q")
	if !p.Paged {
		return p, nil
	}
	if has("limit") {
		n, err := strconv.Atoi(q.Get("limit"))
		if err != nil || n < 1 || n > MaxLimit {
			return p, &ListError{fmt.Sprintf("limit must be a whole number from 1 to %d", MaxLimit)}
		}
		p.Limit = n
	}
	if has("offset") {
		n, err := strconv.Atoi(q.Get("offset"))
		if err != nil || n < 0 {
			return p, &ListError{"offset must be a whole number, 0 or more"}
		}
		p.Offset = n
		if p.Limit == 0 {
			p.Limit = DefaultLimit
		}
	}
	if s := q.Get("sort"); s != "" {
		name := strings.TrimPrefix(s, "-")
		if _, ok := spec.Sortable[name]; !ok {
			return p, &ListError{fmt.Sprintf("sort cannot be %q: use one of %s (with a - before it for descending order)", s, strings.Join(sortNames(spec), ", "))}
		}
		p.Sort, p.Desc = name, strings.HasPrefix(s, "-")
	}
	p.Q = strings.TrimSpace(q.Get("q"))
	return p, nil
}

func sortNames(spec ListSpec) []string {
	names := make([]string, 0, len(spec.Sortable))
	for n := range spec.Sortable {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// likeEscape is the escape character of the LIKE patterns: the same in the three engines.
const likeEscape = "!"

// LikeEscape is the character a LIKE condition built with LikePattern must name in its ESCAPE clause.
const LikeEscape = likeEscape

// LikePattern is the LIKE pattern that matches the text q anywhere, without regard to case: compare it with LOWER(column) and ESCAPE LikeEscape.
func LikePattern(q string) string { return likePattern(q) }

func likePattern(q string) string {
	r := strings.NewReplacer(likeEscape, likeEscape+likeEscape, "%", likeEscape+"%", "_", likeEscape+"_")
	return "%" + r.Replace(strings.ToLower(q)) + "%"
}

// Filter adds the search q of the parameters to a query: rows where any searchable column contains the text, in any case.
func Filter(db *gorm.DB, p ListParams, spec ListSpec) *gorm.DB {
	if p.Q == "" || len(spec.Searchable) == 0 {
		return db
	}
	conds := make([]string, len(spec.Searchable))
	args := make([]any, len(spec.Searchable))
	for i, c := range spec.Searchable {
		conds[i] = "LOWER(" + c + ") LIKE ? ESCAPE '" + likeEscape + "'"
		args[i] = likePattern(p.Q)
	}
	return db.Where("("+strings.Join(conds, " OR ")+")", args...)
}

// Order adds the sort of the parameters (or the default of the spec) and the tie column.
func Order(db *gorm.DB, p ListParams, spec ListSpec) *gorm.DB {
	name, desc := p.Sort, p.Desc
	if name == "" && spec.Default != "" {
		name, desc = strings.TrimPrefix(spec.Default, "-"), strings.HasPrefix(spec.Default, "-")
	}
	if col, ok := spec.Sortable[name]; ok {
		dir := " ASC"
		if desc {
			dir = " DESC"
		}
		db = db.Order(col + dir)
	}
	if spec.Tie != "" {
		db = db.Order(spec.Tie + " ASC")
	}
	return db
}

// Apply adds the search, the sort and the page of the parameters to a query.
func Apply(db *gorm.DB, p ListParams, spec ListSpec) *gorm.DB {
	db = Order(Filter(db, p, spec), p, spec)
	if p.Limit > 0 {
		db = db.Limit(p.Limit)
	}
	if p.Offset > 0 {
		db = db.Offset(p.Offset)
	}
	return db
}

// List reads the rows of the table of T that match the parameters (and the scopes: the filters the list has besides them) and says
// how many match in all, whatever the page.
func (r *Repo[T]) List(p ListParams, spec ListSpec, scopes ...func(*gorm.DB) *gorm.DB) ([]T, int64, error) {
	var zero T
	base := r.db.Model(&zero)
	for _, sc := range scopes {
		base = sc(base)
	}
	base = Filter(base, p, spec)
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []T
	q := Order(base, p, spec)
	if p.Limit > 0 {
		q = q.Limit(p.Limit)
	}
	if p.Offset > 0 {
		q = q.Offset(p.Offset)
	}
	if err := q.Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// WriteListHeaders sets, for a list that was asked with parameters, X-Total-Count (how many rows match in all) and Link with the
// next and previous pages. A browser can read both because the server lists them in Access-Control-Expose-Headers.
func WriteListHeaders(w http.ResponseWriter, r *http.Request, total int64, p ListParams) {
	if !p.Paged {
		return
	}
	h := w.Header()
	h.Set("X-Total-Count", strconv.FormatInt(total, 10))
	if p.Limit == 0 {
		return
	}
	page := func(offset int) string {
		q := url.Values{}
		for k, v := range r.URL.Query() {
			q[k] = v
		}
		q.Set("limit", strconv.Itoa(p.Limit))
		q.Set("offset", strconv.Itoa(offset))
		return r.URL.Path + "?" + q.Encode()
	}
	var links []string
	if int64(p.Offset+p.Limit) < total {
		links = append(links, fmt.Sprintf("<%s>; rel=\"next\"", page(p.Offset+p.Limit)))
	}
	if p.Offset > 0 {
		prev := p.Offset - p.Limit
		if prev < 0 {
			prev = 0
		}
		links = append(links, fmt.Sprintf("<%s>; rel=\"prev\"", page(prev)))
	}
	if len(links) > 0 {
		h.Set("Link", strings.Join(links, ", "))
	}
}

// Slice returns the page of a list that was filtered and sorted in memory: the items from Offset, at most Limit of them (all when Limit is 0).
// A list of the program that has to merge things that do not live in one table (the parts that come with the program and the user's) uses it
// with ParseList and WriteListHeaders.
func Slice[T any](items []T, p ListParams) []T {
	if p.Offset >= len(items) {
		return []T{}
	}
	items = items[p.Offset:]
	if p.Limit > 0 && p.Limit < len(items) {
		items = items[:p.Limit]
	}
	return items
}
