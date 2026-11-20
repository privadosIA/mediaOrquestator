package servertest

// contract.go: the checker that walks a real answer of the server against its description (openapi.yaml). The
// HTTP-level tests of the server and the tests of the families share it.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// OpenAPIDocument converts the YAML of the description to JSON.
func OpenAPIDocument() ([]byte, error) {
	_, here, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "openapi.yaml"))
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

// What the description (openapi.yaml) says an answer looks like must be what the handlers send (spec:
// api-documentation, "The declared fields cannot drift from the real answers"). The checker below reads the
// description as the server serves it and walks a real answer against it. It is stricter than JSON Schema on
// purpose: an object with fields declared lets no other field through, and an object declared with no fields at
// all is a failure ("any object" is exactly what the description must stop saying). It fails on a keyword it does
// not understand instead of ignoring it, so it can never pass by skipping part of a schema.

// SchemaKeywords are the ones the checker understands; the informative ones change nothing.
var SchemaKeywords = map[string]bool{
	"$ref": true, "type": true, "properties": true, "required": true, "items": true, "additionalProperties": true,
	"nullable": true, "enum": true, "pattern": true,
	"description": true, "format": true, "example": true, "default": true, "title": true,
}

// ContractChecker walks JSON against the OpenAPI description of the server; what differs is collected in Problems.
type ContractChecker struct {
	Doc      map[string]any
	Problems []string
}

// NewContractChecker loads the description embedded in the server, failing the test when it cannot be read.
func NewContractChecker(t *testing.T) *ContractChecker {
	t.Helper()
	raw, err := OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return &ContractChecker{Doc: doc}
}

func (c *ContractChecker) fail(path, format string, args ...any) {
	c.Problems = append(c.Problems, path+": "+fmt.Sprintf(format, args...))
}

// responseSchema is the schema the description gives to the 200 JSON answer of a GET endpoint.
func (c *ContractChecker) responseSchema(path string) (map[string]any, bool) {
	var node any = c.Doc
	for _, k := range []string{"paths", path, "get", "responses", "200"} {
		m, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		node = m[k]
	}
	node = c.Resolve(node)
	for _, k := range []string{"content", "application/json", "schema"} {
		m, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		node = m[k]
	}
	s, ok := node.(map[string]any)
	return s, ok
}

// Resolve follows $ref (only local ones, "#/a/b/c") to the object it names.
func (c *ContractChecker) Resolve(node any) any {
	for i := 0; i < 20; i++ {
		m, ok := node.(map[string]any)
		if !ok {
			return node
		}
		ref, ok := m["$ref"].(string)
		if !ok {
			return node
		}
		node = c.Doc
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			next, ok := node.(map[string]any)
			if !ok {
				return nil
			}
			node = next[part]
		}
	}
	return node
}

// Check walks the value v against the schema node at path ($ is the root) and appends what differs to Problems.
func (c *ContractChecker) Check(schemaNode any, v any, path string) {
	schema, ok := c.Resolve(schemaNode).(map[string]any)
	if !ok {
		c.fail(path, "the description has no schema here")
		return
	}
	for k := range schema {
		if !SchemaKeywords[k] {
			c.fail(path, "the check does not understand the keyword %q of the description", k)
		}
	}
	if v == nil {
		if n, _ := schema["nullable"].(bool); !n {
			c.fail(path, "is null but the description does not say it can be")
		}
		return
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if e == v {
				found = true
			}
		}
		if !found {
			c.fail(path, "value %v is not one of %v", v, enum)
		}
	}
	switch schema["type"] {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			c.fail(path, "is not an object")
			return
		}
		props, _ := schema["properties"].(map[string]any)
		extra, hasExtra := schema["additionalProperties"]
		if len(props) == 0 && !hasExtra {
			c.fail(path, "the description declares an object with no fields (\"any object\")")
			return
		}
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				if _, there := obj[r.(string)]; !there {
					c.fail(path+"."+r.(string), "is required by the description and the answer does not have it")
				}
			}
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch {
			case props[k] != nil:
				c.Check(props[k], obj[k], path+"."+k)
			case extra != nil && extra != false:
				if extra == true {
					continue
				}
				c.Check(extra, obj[k], path+"."+k)
			default:
				c.fail(path+"."+k, "the answer has it and the description does not declare it")
			}
		}
	case "array":
		list, ok := v.([]any)
		if !ok {
			c.fail(path, "is not an array")
			return
		}
		items, ok := schema["items"]
		if !ok {
			c.fail(path, "the description declares an array without its items")
			return
		}
		for i, it := range list {
			c.Check(items, it, fmt.Sprintf("%s[%d]", path, i))
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			c.fail(path, "is not a string")
			break
		}
		if pat, ok := schema["pattern"].(string); ok && !regexp.MustCompile(pat).MatchString(str) {
			c.fail(path, "value %q does not match the pattern %s of the description", str, pat)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			c.fail(path, "is not a boolean")
		}
	case "integer":
		if f, ok := v.(float64); !ok || f != float64(int64(f)) {
			c.fail(path, "is not an integer: %v", v)
		}
	case "number":
		if _, ok := v.(float64); !ok {
			c.fail(path, "is not a number")
		}
	default:
		c.fail(path, "the description gives no type here (%v)", schema["type"])
	}
}

// CheckAnswer walks the JSON of a real answer against the description of an endpoint and returns what differs.
func (c *ContractChecker) CheckAnswer(t *testing.T, endpoint, body string) []string {
	t.Helper()
	schema, ok := c.responseSchema(endpoint)
	if !ok {
		t.Fatalf("the description has no JSON answer for GET %s", endpoint)
	}
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("GET %s did not answer JSON: %v", endpoint, err)
	}
	c.Problems = nil
	c.Check(schema, v, "$")
	return c.Problems
}

// ExpectFits fails the test when the body of GET endpoint differs from the description of that endpoint.
func ExpectFits(t *testing.T, c *ContractChecker, endpoint, body string) {
	t.Helper()
	if problems := c.CheckAnswer(t, endpoint, body); len(problems) > 0 {
		t.Errorf("GET %s differs from its description:\n  %s\nanswer: %.600s", endpoint, strings.Join(problems, "\n  "), body)
	}
}
