package server

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseOpenAPI(t *testing.T) map[string]any {
	t.Helper()
	doc, err := openapiDocument()
	if err != nil {
		t.Fatalf("openapiDocument: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(doc, &out); err != nil {
		t.Fatalf("the description is not valid JSON: %v", err)
	}
	return out
}

func TestOpenAPI_Served(t *testing.T) {
	h := newTestServer(t).Handler()
	rec := do(h, "GET", "/api/openapi.json", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type %q", ct)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.0") {
		t.Fatalf("openapi = %q, want 3.0.x", v)
	}
	if _, ok := doc["paths"].(map[string]any); !ok {
		t.Fatal("no paths")
	}
	schemes := doc["components"].(map[string]any)["securitySchemes"].(map[string]any)
	if _, ok := schemes["bearer"]; !ok {
		t.Fatal("the Bearer security scheme is not declared")
	}
}

func TestOpenAPI_StreamsAreDescribed(t *testing.T) {
	doc := parseOpenAPI(t)
	for _, p := range []string{"/api/runs/{id}/events", "/api/downloads/{id}/events"} {
		get := doc["paths"].(map[string]any)[p].(map[string]any)["get"].(map[string]any)
		content := get["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)
		if _, ok := content["text/event-stream"]; !ok {
			t.Errorf("%s: no text/event-stream response", p)
		}
		if !strings.Contains(get["description"].(string), "EventSource") {
			t.Errorf("%s: description does not mention EventSource", p)
		}
	}
}

func TestOpenAPI_RequiresTokenAndHasNoPublicRoute(t *testing.T) {
	h := newAuthServer(t).Handler()
	rec := do(h, "GET", "/api/openapi.json", nil)
	if rec.Code != 401 {
		t.Fatalf("anonymous: status %d, want 401", rec.Code)
	}
	if decodeAPIError(t, rec.Body.String()) != codeUnauthorized {
		t.Error("wrong error code")
	}
	if strings.Contains(rec.Body.String(), "openapi") {
		t.Error("the 401 body leaks the description")
	}
	if rec := do(h, "GET", "/api/openapi.json", map[string]string{"Authorization": "Bearer " + testToken}); rec.Code != 200 {
		t.Fatalf("with token: status %d", rec.Code)
	}
	for _, p := range []string{"/swagger", "/swagger/", "/docs", "/openapi.json", "/swagger.json"} {
		rec := do(h, "GET", p, nil)
		if strings.Contains(rec.Body.String(), `"openapi"`) || strings.Contains(strings.ToLower(rec.Body.String()), "swagger-ui") {
			t.Errorf("%s serves the description or Swagger UI", p)
		}
	}
}

// documentedRoutes returns the "METHOD /path" set of the embedded YAML.
func documentedRoutes(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(openapiYAML, &doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	for path, ops := range doc.Paths {
		for m := range ops {
			switch m {
			case "get", "post", "put", "delete", "patch":
				out = append(out, strings.ToUpper(m)+" "+path)
			}
		}
	}
	sort.Strings(out)
	return out
}

// registeredRoutes normalises the patterns recorded by Server.handle:
// "{path...}" becomes "{path}", and the catch-alls are left out.
func registeredRoutes(s *Server) []string {
	var out []string
	for _, p := range s.patterns {
		if !strings.Contains(p, " ") {
			continue
		}
		out = append(out, strings.ReplaceAll(p, "...}", "}"))
	}
	sort.Strings(out)
	return out
}

func diffRoutes(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	return out
}

func TestOpenAPI_MatchesRegisteredRoutes(t *testing.T) {
	registered := registeredRoutes(newTestServer(t))
	documented := documentedRoutes(t)
	if missing := diffRoutes(registered, documented); len(missing) > 0 {
		t.Errorf("registered but not in openapi.yaml: %v", missing)
	}
	if extra := diffRoutes(documented, registered); len(extra) > 0 {
		t.Errorf("in openapi.yaml but not registered: %v", extra)
	}
}

// ticketPaths returns the paths of the description whose GET accepts the ticket, with {x} replaced by a value.
func ticketPaths(t *testing.T) map[string]bool {
	t.Helper()
	doc0, err := openapiDocument()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(doc0, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for p, ops := range doc.Paths {
		raw, ok := ops["get"]
		if !ok {
			continue
		}
		var get struct {
			Security []map[string][]string `json:"security"`
		}
		if err := json.Unmarshal(raw, &get); err != nil {
			t.Fatal(err)
		}
		declared := false
		for _, req := range get.Security {
			if _, has := req["ticket"]; has {
				declared = true
			}
		}
		out[p] = declared
	}
	return out
}

// TestOpenAPI_TicketRoutesAreDeclared: the routes that accept a ticket are the ones whose description declares the
// ticket security scheme, no more and no fewer.
func TestOpenAPI_TicketRoutesAreDeclared(t *testing.T) {
	re := regexp.MustCompile(`\{[^}]+\}`)
	declared := ticketPaths(t)
	accepts := 0
	for p, has := range declared {
		req := httptest.NewRequest("GET", re.ReplaceAllString(p, "x"), nil)
		serves := ticketRoute(req)
		if serves {
			accepts++
		}
		switch {
		case serves && !has:
			t.Errorf("%s accepts a ticket but its description does not declare the ticket scheme", p)
		case !serves && has:
			t.Errorf("%s declares the ticket scheme but the server does not accept a ticket there", p)
		}
	}
	if accepts != len(ticketRoutes) {
		t.Errorf("the description covers %d of the %d ticket routes of the server", accepts, len(ticketRoutes))
	}
}
