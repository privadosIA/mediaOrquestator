package hyperframes

import (
	"bytes"
	"strings"
	"testing"
)

func TestReportClassifiesAndOrders(t *testing.T) {
	bs := []Block{
		{Name: "a-ok", Checks: &CheckResult{OK: true}},
		{Name: "b-files", Checks: &CheckResult{Problems: []string{"x"}, MissingFiles: []string{"assets/a.jpg"}}},
		{Name: "c-gpu", Checks: &CheckResult{Problems: []string{"WebGPU unavailable"}, NeedsGPU: true}},
		{Name: "d-bad", Checks: &CheckResult{Problems: []string{"nothing moves"}}},
		{Name: "e-new"},
	}
	r := BuildReport(bs)
	if r.Total != 5 || r.ByState["ok"] != 1 || r.ByState["needs-files"] != 1 || r.ByState["needs-gpu"] != 1 || r.ByState["failed"] != 1 || r.ByState["unchecked"] != 1 {
		t.Fatalf("%+v", r.ByState)
	}
	if r.Lines[0].Name != "d-bad" || r.Lines[len(r.Lines)-1].Name != "a-ok" {
		t.Fatalf("order: %+v", r.Lines)
	}
	var buf bytes.Buffer
	r.WriteText(&buf)
	out := buf.String()
	if strings.Contains(out, "a-ok") || !strings.Contains(out, "d-bad") || !strings.Contains(out, "5 block(s): 1 ok") {
		t.Fatalf("%s", out)
	}
}
