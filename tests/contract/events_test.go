package contract_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

const (
	eventsDir = "../../events"
	idPrefix  = "https://schemas.synthwerk.dev/events/"
)

var typeRule = regexp.MustCompile(`^dev\.synthwerk\.([a-z][a-z0-9]*)\.([a-z][a-z0-9_]*)\.([a-z][a-z0-9_]*)\.(v[1-9][0-9]*)$`)

type catalogEntry struct {
	Type      string   `yaml:"type"`
	NATSSubj  string   `yaml:"nats_subject"`
	Stream    string   `yaml:"stream"`
	Producer  string   `yaml:"producer"`
	Consumers []string `yaml:"consumers"`
	Status    string   `yaml:"status"`
	Schema    string   `yaml:"schema"`
	Example   string   `yaml:"example"`
}

func loadCatalog(t *testing.T) []catalogEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(eventsDir, "catalog.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Events []catalogEntry `yaml:"events"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&catalog); err != nil {
		t.Fatalf("catalog.yaml: %v", err)
	}
	if len(catalog.Events) == 0 {
		t.Fatal("catalog.yaml: no events")
	}
	return catalog.Events
}

func compile(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile(abs)
	if err != nil {
		t.Fatalf("compile %s: %v", path, err)
	}
	return s
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	f, err := os.Open(path) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	v, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: not a JSON object", path)
	}
	return m
}

func TestCatalog(t *testing.T) {
	envelope := compile(t, filepath.Join(eventsDir, "envelope.schema.json"))

	for _, e := range loadCatalog(t) {
		t.Run(e.Type, func(t *testing.T) {
			m := typeRule.FindStringSubmatch(e.Type)
			if m == nil {
				t.Fatalf("type %q breaks the rule dev.synthwerk.<domain>.<entity>.<verb>.v<N>", e.Type)
			}
			domain, entity, verb, version := m[1], m[2], m[3], m[4]

			// Names derive from the type (docs/naming.md).
			checkEqual(t, "nats_subject", e.NATSSubj, "evt."+strings.TrimPrefix(e.Type, "dev.synthwerk."))
			checkEqual(t, "stream", e.Stream, "EVT_"+strings.ToUpper(domain))
			checkEqual(t, "producer", e.Producer, domain)
			checkEqual(t, "schema", e.Schema, domain+"/"+entity+"."+verb+"."+version+".schema.json")
			checkEqual(t, "example", e.Example, "examples/"+e.Type+".json")
			if !slices.Contains([]string{"draft", "active", "deprecated", "retired"}, e.Status) {
				t.Errorf("status %q: want draft, active, deprecated or retired", e.Status)
			}

			schemaPath := filepath.Join(eventsDir, e.Schema)
			wantID := idPrefix + strings.TrimSuffix(e.Schema, ".schema.json") + ".json"
			checkEqual(t, "schema $id", readJSON(t, schemaPath)["$id"], wantID)
			data := compile(t, schemaPath)

			example := readJSON(t, filepath.Join(eventsDir, e.Example))
			if err := envelope.Validate(example); err != nil {
				t.Fatalf("example breaks the envelope: %v", err)
			}
			checkEqual(t, "example type", example["type"], e.Type)
			if ds, ok := example["dataschema"]; ok {
				checkEqual(t, "example dataschema", ds, wantID)
			}
			if err := data.Validate(example["data"]); err != nil {
				t.Fatalf("example data breaks %s: %v", e.Schema, err)
			}
		})
	}
}

func TestCatalog_everySchemaListed(t *testing.T) {
	listed := map[string]bool{}
	for _, e := range loadCatalog(t) {
		listed[e.Schema] = true
	}
	err := filepath.WalkDir(eventsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".schema.json") {
			return err
		}
		rel, _ := filepath.Rel(eventsDir, path)
		rel = filepath.ToSlash(rel)
		if rel != "envelope.schema.json" && !listed[rel] {
			t.Errorf("%s: not in catalog.yaml", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The schemas reject what a typo or a wrong producer would send.
func TestSchemas_reject(t *testing.T) {
	envelope := compile(t, filepath.Join(eventsDir, "envelope.schema.json"))
	toolcall := compile(t, filepath.Join(eventsDir, "llm/toolcall.executed.v1.schema.json"))
	base := readJSON(t, filepath.Join(eventsDir, "examples/dev.synthwerk.llm.toolcall.executed.v1.json"))

	tests := []struct {
		name   string
		schema *jsonschema.Schema
		edit   func(e map[string]any) any
	}{
		{"envelope - type without version suffix - rejected", envelope, func(e map[string]any) any {
			e["type"] = "dev.synthwerk.llm.toolcall.executed"
			return e
		}},
		{"envelope - id is a UUIDv4 - rejected", envelope, func(e map[string]any) any {
			e["id"] = "6f1c2d3e-4a5b-4c6d-8e7f-9a0b1c2d3e4f"
			return e
		}},
		{"envelope - extension name in camelCase - rejected", envelope, func(e map[string]any) any {
			e["orgId"] = e["orgid"]
			delete(e, "orgid")
			return e
		}},
		{"envelope - actor is an email - rejected", envelope, func(e map[string]any) any {
			e["actor"] = "user:ada@example.com"
			return e
		}},
		{"toolcall data - tool arguments added - rejected", toolcall, func(e map[string]any) any {
			data := e["data"].(map[string]any)
			data["arguments"] = map[string]any{"query": "all users"}
			return data
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.schema.Validate(tt.edit(clone(t, base))); err == nil {
				t.Fatal("validation passed, want an error")
			}
		})
	}
}

func clone(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func checkEqual(t *testing.T, what string, got, want any) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", what, got, want)
	}
}
