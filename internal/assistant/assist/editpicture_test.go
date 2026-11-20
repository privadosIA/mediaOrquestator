package assist

import "testing"

func TestEditInstruction(t *testing.T) {
	cases := map[string]string{
		"Make the hair of the man on the left blonde":                  "Make the hair of the man on the left blonde. Keep everything else exactly the same.",
		"\"Put a red cap on the woman on the right.\"":                 "Put a red cap on the woman on the right. Keep everything else exactly the same.",
		"Make his hair blonde. Keep everything else exactly the same.": "Make his hair blonde. Keep everything else exactly the same.",
		"Transform the picture into: a snowy street at night":          "Transform the picture into: a snowy street at night",
		"   ": "",
	}
	for in, want := range cases {
		if got := EditInstruction(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestEditTaskIsRegisteredButNotOffered(t *testing.T) {
	if _, ok := tasks[TaskEdit]; !ok {
		t.Fatal("the edit task is registered")
	}
	if _, err := ParseTask("edit"); err == nil {
		t.Fatal("the edit task must not be offered through ParseTask")
	}
}
