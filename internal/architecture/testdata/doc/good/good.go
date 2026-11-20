// Package good is documented the way the test wants.
package good

import "errors"

// Thing is a type with a comment that starts with its name.
type Thing struct{}

// A Widget may start with an article.
type Widget struct{}

// Make builds a Thing.
func Make() Thing { return Thing{} }

// Do is a method of an exported type.
func (Thing) Do() {}

// Limits of the thing.
const (
	Low  = 1
	High = 2
)

// ErrNone is a documented variable.
var ErrNone = errors.New("none")

// E is an error type: its methods implement well-known interfaces and need no comment.
type E struct{}

func (E) Error() string { return "e" }

type private struct{}

// Exported on an unexported receiver needs nothing.
func (private) Visible() {}

func unexported() {}
