package eventcompat_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/VelimirMueller/synthwerk-contracts/tools/eventcompat"
)

// One case per rule in docs/features/breaking-checks.md.
func TestCompare(t *testing.T) {
	const base = `{
		"type": "object",
		"required": ["target", "to"],
		"properties": {
			"target": {"type": "string", "maxLength": 64},
			"to": {"enum": ["ok", "degraded", "down"]},
			"note": {"type": "string"},
			"checks": {"type": "array", "items": {"type": "object", "required": ["name"], "properties": {"name": {"type": "string"}}}}
		}
	}`
	tests := []struct {
		name string
		edit func(s map[string]any)
		want []string // findings as "SEVERITY path: message"; empty = compatible
	}{
		{
			name: "Compare - optional property added - no finding",
			edit: func(s map[string]any) { props(s)["since"] = map[string]any{"type": "string"} },
		},
		{
			name: "Compare - maxLength raised - no finding",
			edit: func(s map[string]any) { props(s)["target"].(map[string]any)["maxLength"] = 128.0 },
		},
		{
			name: "Compare - required property renamed - breaking",
			edit: func(s map[string]any) {
				props(s)["service"] = props(s)["target"]
				delete(props(s), "target")
				s["required"] = []any{"service", "to"}
			},
			want: []string{
				"BREAKING target: required property removed",
				"BREAKING service: new required property",
			},
		},
		{
			name: "Compare - optional property removed - breaking",
			edit: func(s map[string]any) { delete(props(s), "note") },
			want: []string{"BREAKING note: property removed"},
		},
		{
			name: "Compare - optional property made required - breaking",
			edit: func(s map[string]any) { s["required"] = []any{"target", "to", "note"} },
			want: []string{"BREAKING note: property became required"},
		},
		{
			name: "Compare - required property made optional - breaking",
			edit: func(s map[string]any) { s["required"] = []any{"to"} },
			want: []string{"BREAKING target: property no longer required: consumers can miss it"},
		},
		{
			name: "Compare - type changed - breaking",
			edit: func(s map[string]any) { props(s)["target"].(map[string]any)["type"] = "integer" },
			want: []string{"BREAKING target: type changed from [string] to [integer]"},
		},
		{
			name: "Compare - format added - breaking",
			edit: func(s map[string]any) { props(s)["target"].(map[string]any)["format"] = "uuid" },
			want: []string{"BREAKING target: format changed from <nil> to uuid"},
		},
		{
			name: "Compare - enum value removed - breaking",
			edit: func(s map[string]any) { props(s)["to"].(map[string]any)["enum"] = []any{"ok", "down"} },
			want: []string{"BREAKING to: enum value degraded removed"},
		},
		{
			name: "Compare - enum value added - warning only",
			edit: func(s map[string]any) {
				props(s)["to"].(map[string]any)["enum"] = []any{"ok", "degraded", "down", "unknown"}
			},
			want: []string{"WARN to: enum value unknown added: consumers must tolerate it"},
		},
		{
			name: "Compare - nested item property renamed - breaking",
			edit: func(s map[string]any) {
				item := props(s)["checks"].(map[string]any)["items"].(map[string]any)
				item["properties"] = map[string]any{"id": map[string]any{"type": "string"}}
				item["required"] = []any{"id"}
			},
			want: []string{
				"BREAKING checks[].name: required property removed",
				"BREAKING checks[].id: new required property",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, h := decode(t, base), decode(t, base)
			tt.edit(h)

			var got []string
			for _, f := range eventcompat.Compare(b, h) {
				got = append(got, f.String())
			}
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestCompareDirs(t *testing.T) {
	t.Run("CompareDirs - schema file removed - breaking", func(t *testing.T) {
		base, head := t.TempDir(), t.TempDir()
		write(t, filepath.Join(base, "pulse", "target.status_changed.v1.schema.json"), `{"type": "object"}`)

		findings, err := eventcompat.CompareDirs(base, head)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].String() != "BREAKING pulse/target.status_changed.v1.schema.json (root): schema removed" {
			t.Fatalf("findings: %v", findings)
		}
	})

	t.Run("CompareDirs - base has no events dir - no finding", func(t *testing.T) {
		findings, err := eventcompat.CompareDirs(filepath.Join(t.TempDir(), "events"), t.TempDir())
		if err != nil || len(findings) != 0 {
			t.Fatalf("findings: %v, err: %v", findings, err)
		}
	})

	t.Run("CompareDirs - repo schemas against themselves - no finding", func(t *testing.T) {
		findings, err := eventcompat.CompareDirs("../../../events", "../../../events")
		if err != nil || len(findings) != 0 {
			t.Fatalf("findings: %v, err: %v", findings, err)
		}
	})
}

func props(s map[string]any) map[string]any { return s["properties"].(map[string]any) }

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var s map[string]any
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
