package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Text is a column that holds long text. MySQL's plain text stops at 64 KB, and a page of a part or the map of a song is
// larger: there it is longtext; SQLite and Postgres have no such limit in text.
type Text string

// GormDataType is the name GORM knows the type by.
func (Text) GormDataType() string { return "text" }

// GormDBDataType gives the column type of each engine.
func (Text) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db.Dialector.Name() == "mysql" {
		return "longtext"
	}
	return "text"
}

// JSON is a column that holds a value of any type as JSON text, so that a document is stored whole and nothing of it is
// queried (a query is made on a scalar column: the JSON functions differ in each engine). NULL and empty text read as the
// zero value.
type JSON[T any] struct{ V T }

// GormDataType is the name GORM knows the type by.
func (JSON[T]) GormDataType() string { return "text" }

// GormDBDataType gives the column type of each engine.
func (JSON[T]) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	return Text("").GormDBDataType(db, nil)
}

// Value writes the value as JSON.
func (j JSON[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(j.V)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan reads the value from JSON.
func (j *JSON[T]) Scan(src any) error {
	var b []byte
	switch v := src.(type) {
	case nil:
		var zero T
		j.V = zero
		return nil
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		return fmt.Errorf("store: cannot read a JSON column from %T", src)
	}
	if len(b) == 0 {
		var zero T
		j.V = zero
		return nil
	}
	return json.Unmarshal(b, &j.V)
}

// MarshalJSON writes the wrapped value, so that a struct that holds a JSON column is the same JSON as one that holds the value.
func (j JSON[T]) MarshalJSON() ([]byte, error) { return json.Marshal(j.V) }

// UnmarshalJSON reads the wrapped value.
func (j *JSON[T]) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &j.V) }
