package store

import (
	"strings"
	"testing"
)

type typesProbe struct {
	ID   uint `gorm:"primaryKey"`
	Doc  JSON[map[string]int]
	Body Text
}

func TestJSONAndTextRoundTripThroughTheDatabase(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s, err := OpenMemory(nil)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if err := s.DB.AutoMigrate(&typesProbe{}); err != nil {
			t.Fatal(err)
		}
		big := strings.Repeat("x", 200*1024)
		in := typesProbe{Doc: JSON[map[string]int]{V: map[string]int{"a": 1, "b": 2}}, Body: Text(big)}
		if err := s.DB.Create(&in).Error; err != nil {
			t.Fatal(err)
		}
		var out typesProbe
		if err := s.DB.First(&out, in.ID).Error; err != nil {
			t.Fatal(err)
		}
		if out.Doc.V["a"] != 1 || out.Doc.V["b"] != 2 || len(out.Doc.V) != 2 {
			t.Errorf("doc = %v", out.Doc.V)
		}
		if string(out.Body) != big {
			t.Errorf("body of %d bytes came back as %d", len(big), len(out.Body))
		}
	})
}

func TestANullOrEmptyJSONColumnIsTheZeroValue(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s, err := OpenMemory(nil)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if err := s.DB.AutoMigrate(&typesProbe{}); err != nil {
			t.Fatal(err)
		}
		if err := s.DB.Exec("INSERT INTO types_probes (id, doc, body) VALUES (1, NULL, ''), (2, '', '')").Error; err != nil {
			t.Fatal(err)
		}
		for _, id := range []uint{1, 2} {
			var out typesProbe
			if err := s.DB.First(&out, id).Error; err != nil {
				t.Fatal(err)
			}
			if out.Doc.V != nil {
				t.Errorf("row %d: %v", id, out.Doc.V)
			}
		}
	})
}

func TestTheColumnTypeOfTextFollowsTheEngine(t *testing.T) {
	eachEngine(t, func(t *testing.T) {
		s, err := OpenMemory(nil)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		want := "text"
		if s.Driver == "mysql" {
			want = "longtext" // MySQL's text stops at 64 KB
		}
		if got := (Text("")).GormDBDataType(s.DB, nil); got != want {
			t.Errorf("%s: %q, want %q", s.Driver, got, want)
		}
	})
}
