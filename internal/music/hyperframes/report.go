package hyperframes

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// ReportLine is what the report says about one block.
type ReportLine struct {
	Name string `json:"name"`
	// State is ok, needs-files, needs-gpu, failed or unchecked.
	State    string   `json:"state"`
	Problems []string `json:"problems,omitempty"`
}

// Report is the outcome of the checks over the blocks of a store.
type Report struct {
	Total int `json:"total"`
	// ByState counts the blocks of each state.
	ByState map[string]int `json:"by_state"`
	Lines   []ReportLine   `json:"lines"`
}

// StateOf classifies a block by its last check.
func StateOf(b Block) string {
	c := b.Checks
	switch {
	case c == nil:
		return "unchecked"
	case c.OK:
		return "ok"
	case len(c.MissingFiles) > 0:
		return "needs-files"
	case c.NeedsGPU:
		return "needs-gpu"
	}
	return "failed"
}

// BuildReport lists every block with its state, the ones that do not draw first.
func BuildReport(blocks []Block) Report {
	r := Report{Total: len(blocks), ByState: map[string]int{}}
	for _, b := range blocks {
		l := ReportLine{Name: b.Name, State: StateOf(b)}
		if b.Checks != nil {
			l.Problems = b.Checks.Problems
		}
		r.ByState[l.State]++
		r.Lines = append(r.Lines, l)
	}
	rank := map[string]int{"failed": 0, "needs-gpu": 1, "needs-files": 2, "unchecked": 3, "ok": 4}
	sort.SliceStable(r.Lines, func(i, j int) bool {
		if r.Lines[i].State != r.Lines[j].State {
			return rank[r.Lines[i].State] < rank[r.Lines[j].State]
		}
		return r.Lines[i].Name < r.Lines[j].Name
	})
	return r
}

// WriteText prints the report: one line per block that is not ok, and the totals.
func (r Report) WriteText(w io.Writer) {
	for _, l := range r.Lines {
		if l.State == "ok" {
			continue
		}
		fmt.Fprintf(w, "%-34s %-11s %s\n", l.Name, l.State, strings.Join(l.Problems, "; "))
	}
	fmt.Fprintf(w, "%d block(s): %d ok, %d need files the registry does not ship, %d need a GPU, %d failed, %d not checked\n",
		r.Total, r.ByState["ok"], r.ByState["needs-files"], r.ByState["needs-gpu"], r.ByState["failed"], r.ByState["unchecked"])
}
