// Package edition is the one place where the private and the public editions of
// the program differ. The private edition is built with the Go build tag
// `private`, the public one without it, and nothing at run time (setting,
// environment variable, request, file) can change which one a binary is.
//
// Callers never look at the tag: they call this package. What exists only in
// the private edition lives in edition_private.go, which the public branch does
// not contain (see scripts/sync-public.sh), so the public tree builds with the
// safe twin in edition_public.go.
//
// The check that stops describing a minor is NOT here: it is the same code in
// both editions and has no switch.
package edition

import (
	"errors"
	"fmt"
)

// ErrModelNotAllowed marks a model that this edition does not use.
var ErrModelNotAllowed = errors.New("this edition does not use that model")

// NotAllowedError is the error of DenyModel; it matches ErrModelNotAllowed.
type NotAllowedError struct{ Model string }

func (e *NotAllowedError) Error() string {
	return fmt.Sprintf("the model %q is not used by this edition of the program", e.Model)
}

func (e *NotAllowedError) Is(target error) bool { return target == ErrModelNotAllowed }
