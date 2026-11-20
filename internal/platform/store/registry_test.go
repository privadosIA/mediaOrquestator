package store

import "testing"

type regA struct{ ID uint }
type regB struct{ ID uint }

func TestModelsComeBackInTheOrderTheyWereRegistered(t *testing.T) {
	r := NewRegistry()
	r.AddModels(&regB{})
	r.AddModels(&regA{})
	got := r.Models()
	// a new registry already has what this package owns: the records of the imports
	if len(got) != 3 {
		t.Fatalf("%d models", len(got))
	}
	if _, ok := got[0].(*DataImport); !ok {
		t.Errorf("first = %T, want *DataImport", got[0])
	}
	if _, ok := got[1].(*regB); !ok {
		t.Errorf("second = %T, want *regB", got[1])
	}
	if _, ok := got[2].(*regA); !ok {
		t.Errorf("third = %T, want *regA", got[2])
	}
	if m := r.Migrations(); len(m) != 1 || m[0].ID != "202611181210_data_imports" {
		t.Errorf("migrations = %v", m)
	}
}

func TestAModelRegisteredTwiceIsAPanic(t *testing.T) {
	r := NewRegistry()
	r.AddModels(&regA{})
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	r.AddModels(&regA{})
}

func TestOnlyPointersToStructsCanBeRegistered(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	r.AddModels(regA{})
}

func TestTwoRegistriesDoNotShareAnything(t *testing.T) {
	a, b := NewRegistry(), NewRegistry()
	a.AddModels(&regA{})
	a.AddImporter(&fakeImporter{name: "x"})
	if len(b.Models()) != 1 || len(b.Importers()) != 0 {
		t.Fatalf("a registry must not see what another one was given: %d models, %d importers", len(b.Models()), len(b.Importers()))
	}
}

func TestAnImporterAddedTwiceIsAPanic(t *testing.T) {
	r := NewRegistry()
	r.AddImporter(&fakeImporter{name: "x"})
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	r.AddImporter(&fakeImporter{name: "x"})
}
