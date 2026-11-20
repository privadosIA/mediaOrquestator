package musicmotion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrPartNotFound is a name that is not a part of the catalog.
	ErrPartNotFound = errors.New("part not found")
	// ErrBuiltinPart is a part that comes with the program: it cannot be deleted or replaced.
	ErrBuiltinPart = errors.New("a part that comes with the program cannot be changed")
	// ErrPartRenamed is a replacement whose manifest has another name than the part's.
	ErrPartRenamed = errors.New("the manifest names another part")
)

// UsesPart says whether a script draws the part: as the main part or an extra of a piece, or in an item of a layer.
func (s Script) UsesPart(name string) bool {
	for _, p := range s.Pieces {
		if p.Part == name {
			return true
		}
		for _, w := range p.With {
			if w.Part == name {
				return true
			}
		}
	}
	for _, l := range s.Layers {
		for _, it := range l.Items {
			if it.Part == name {
				return true
			}
		}
	}
	return false
}

// PartNames lists, once each and in the order they first appear, the parts a script draws: the main part and the extras of every piece and the
// parts of the items of its layers. A reader that needs them all asks the catalog for them with one GetMany.
func (s Script) PartNames() []string {
	var out []string
	seen := map[string]bool{}
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, p := range s.Pieces {
		add(p.Part)
		for _, w := range p.With {
			add(w.Part)
		}
	}
	for _, l := range s.Layers {
		for _, it := range l.Items {
			add(it.Part)
		}
	}
	return out
}

// checkUserPart says the part is one of the user's: ErrPartNotFound when there is none and ErrBuiltinPart for one that comes with the program.
func (c Catalog) checkUserPart(name string) error {
	p, ok := c.Get(name)
	if !ok {
		return ErrPartNotFound
	}
	if p.Source != "user" || c.userStore() == nil {
		return ErrBuiltinPart
	}
	return nil
}

// CreatePart keeps a new part of the user's (ErrPartExists when one has its name, or one that comes with the program).
func (c Catalog) CreatePart(manifest []byte, fragment string, o *PartOrigin) error {
	st := c.userStore()
	if st == nil {
		return errors.New("this catalog has nowhere to keep the user's parts")
	}
	var head struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(manifest, &head); err == nil {
		if _, builtin := c.builtinParts()[head.Name]; builtin {
			return fmt.Errorf("%w: %q is the name of a part that comes with the program", ErrPartExists, head.Name)
		}
	}
	return st.Create(manifest, fragment, o)
}

// ReplacePart changes the manifest and the fragment of a part of the user's (its origin stays). The catalog shows the new one at once.
func (c Catalog) ReplacePart(name string, manifest []byte, fragment string) error {
	if err := c.checkUserPart(name); err != nil {
		return err
	}
	return c.userStore().Replace(name, manifest, fragment)
}

// DeleteUserPart removes a part of the user's. The catalog stops listing it at once.
func (c Catalog) DeleteUserPart(name string) error {
	if err := c.checkUserPart(name); err != nil {
		return err
	}
	return c.userStore().Delete(name)
}

// ReplaceUserPart checks a new manifest and fragment like an approved draft is checked and, if they pass, writes them
// over the part of the user's with the same name (its origin stays). The problems are returned and nothing changes when
// they do not pass.
func (e *DraftEngine) ReplaceUserPart(ctx context.Context, name, manifest, fragment string) (Part, []string, error) {
	if err := e.Cat.checkUserPart(name); err != nil {
		return Part{}, nil, err
	}
	part, probs := ParsePart([]byte(manifest), fragment)
	if len(probs) == 0 && part.Name != name {
		return Part{}, nil, fmt.Errorf("%w: %q instead of %q", ErrPartRenamed, part.Name, name)
	}
	if len(probs) == 0 {
		var err error
		if probs, err = CheckPart(ctx, e.Browser, part, CheckOptions{}); err != nil {
			return Part{}, nil, err
		}
	}
	if len(probs) > 0 {
		return Part{}, probs, nil
	}
	if err := e.Cat.ReplacePart(name, []byte(manifest), fragment); err != nil {
		return Part{}, nil, err
	}
	got, _ := e.Cat.Get(name)
	return got, nil, nil
}

// DeleteScenes removes the scene parts written for a piece (when the piece is deleted) and returns how many.
func (c Catalog) DeleteScenes(piece string) int {
	st := c.userStore()
	if st == nil {
		return 0
	}
	n, _ := st.DeleteScenes(piece)
	return n
}

// CreateUserPart checks a manifest and a fragment like an approved draft is checked and, if they pass, keeps them as a new part of the
// user's. The problems are returned and nothing is kept when they do not pass; ErrNameTaken when the catalog has a part of that name.
func (e *DraftEngine) CreateUserPart(ctx context.Context, manifest, fragment string) (Part, []string, error) {
	part, probs := ParsePart([]byte(manifest), fragment)
	if len(probs) > 0 {
		return Part{}, probs, nil
	}
	if _, taken := e.Cat.Get(part.Name); taken {
		return Part{}, nil, ErrNameTaken
	}
	probs, err := CheckPart(ctx, e.Browser, part, CheckOptions{})
	if err != nil {
		return Part{}, nil, err
	}
	if len(probs) > 0 {
		return Part{}, probs, nil
	}
	if err := e.Cat.CreatePart([]byte(manifest), fragment, nil); err != nil {
		if errors.Is(err, ErrPartExists) {
			return Part{}, nil, ErrNameTaken
		}
		return Part{}, nil, err
	}
	got, _ := e.Cat.Get(part.Name)
	return got, nil, nil
}
