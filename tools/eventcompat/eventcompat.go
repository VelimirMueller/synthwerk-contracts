// Package eventcompat compares two versions of the event JSON Schemas and reports
// the changes that break a consumer. The rules are in docs/features/breaking-checks.md.
package eventcompat

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// Severity tells if a finding fails the check.
type Severity string

const (
	// Breaking fails the check. Publish a new type .v<N+1> instead.
	Breaking Severity = "BREAKING"
	// Warning passes. Consumers must tolerate the change (for example a new enum value).
	Warning Severity = "WARN"
)

// Finding is one change between the base and the head schema.
type Finding struct {
	Severity Severity
	File     string // schema path relative to the events dir; empty for Compare
	Path     string // field path, for example "checks[].name"; "(root)" for the schema itself
	Message  string
}

func (f Finding) String() string {
	if f.File == "" {
		return fmt.Sprintf("%s %s: %s", f.Severity, f.Path, f.Message)
	}
	return fmt.Sprintf("%s %s %s: %s", f.Severity, f.File, f.Path, f.Message)
}

// HasBreaking reports whether any finding is breaking.
func HasBreaking(findings []Finding) bool {
	return slices.ContainsFunc(findings, func(f Finding) bool { return f.Severity == Breaking })
}

// Compare returns the findings for one schema. base is the published version, head the new one.
func Compare(base, head map[string]any) []Finding {
	var c comparer
	c.node("", base, head)
	return c.findings
}

// CompareDirs compares every *.schema.json under baseDir with the file at the same path under headDir.
// A missing baseDir means nothing is published yet: no findings.
func CompareDirs(baseDir, headDir string) ([]Finding, error) {
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil, nil
	}
	var files []string
	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".schema.json") {
			rel, err := filepath.Rel(baseDir, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	var findings []Finding
	for _, rel := range files {
		headPath := filepath.Join(headDir, filepath.FromSlash(rel))
		if _, err := os.Stat(headPath); os.IsNotExist(err) {
			findings = append(findings, Finding{Severity: Breaking, File: rel, Path: "(root)", Message: "schema removed"})
			continue
		}
		base, err := readSchema(filepath.Join(baseDir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		head, err := readSchema(headPath)
		if err != nil {
			return nil, err
		}
		for _, f := range Compare(base, head) {
			f.File = rel
			findings = append(findings, f)
		}
	}
	return findings, nil
}

func readSchema(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path comes from a walk of the given schema dir
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return schema, nil
}

type comparer struct {
	findings []Finding
}

func (c *comparer) add(sev Severity, path, format string, args ...any) {
	if path == "" {
		path = "(root)"
	}
	c.findings = append(c.findings, Finding{Severity: sev, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (c *comparer) node(path string, base, head map[string]any) {
	if bt, ht := typeSet(base), typeSet(head); !slices.Equal(bt, ht) {
		c.add(Breaking, path, "type changed from %v to %v", bt, ht)
	}
	// prefixItems (tuples) is compared as a whole: any change counts as breaking.
	for _, key := range []string{"format", "const", "$ref", "prefixItems"} {
		if !reflect.DeepEqual(base[key], head[key]) {
			c.add(Breaking, path, "%s changed from %v to %v", key, base[key], head[key])
		}
	}
	c.enum(path, base, head)
	c.properties(path, base, head)
	if bi, ok := base["items"].(map[string]any); ok {
		if hi, ok := head["items"].(map[string]any); ok {
			c.node(path+"[]", bi, hi)
		} else {
			c.add(Breaking, path, "items schema removed")
		}
	}
}

func (c *comparer) enum(path string, base, head map[string]any) {
	be, ok := base["enum"].([]any)
	if !ok {
		return
	}
	he, ok := head["enum"].([]any)
	if !ok {
		// Same consumer risk as a new enum value (plan §5.4: warn), only wider.
		c.add(Warning, path, "enum removed: consumers can get any value")
		return
	}
	for _, v := range be {
		if !containsValue(he, v) {
			c.add(Breaking, path, "enum value %v removed", v)
		}
	}
	for _, v := range he {
		if !containsValue(be, v) {
			c.add(Warning, path, "enum value %v added: consumers must tolerate it", v)
		}
	}
}

func (c *comparer) properties(path string, base, head map[string]any) {
	bp, _ := base["properties"].(map[string]any)
	hp, _ := head["properties"].(map[string]any)
	br, hr := stringSet(base["required"]), stringSet(head["required"])

	for _, name := range sortedKeys(bp) {
		child := join(path, name)
		hs, ok := hp[name]
		if !ok {
			if br[name] {
				c.add(Breaking, child, "required property removed")
			} else {
				c.add(Breaking, child, "property removed")
			}
			continue
		}
		if br[name] && !hr[name] {
			c.add(Breaking, child, "property no longer required: consumers can miss it")
		}
		bs, bok := bp[name].(map[string]any)
		hsm, hok := hs.(map[string]any)
		if bok && hok {
			c.node(child, bs, hsm)
		}
	}
	for _, name := range sortedKeys(hr) {
		if !br[name] {
			if _, existed := bp[name]; existed {
				c.add(Breaking, join(path, name), "property became required")
			} else {
				c.add(Breaking, join(path, name), "new required property")
			}
		}
	}
}

// typeSet returns the sorted JSON types of a schema node. An enum or const without "type"
// gets no type set, so only explicit type changes count.
func typeSet(schema map[string]any) []string {
	var types []string
	switch t := schema["type"].(type) {
	case string:
		types = []string{t}
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok {
				types = append(types, s)
			}
		}
	}
	sort.Strings(types)
	return types
}

func stringSet(v any) map[string]bool {
	set := map[string]bool{}
	if list, ok := v.([]any); ok {
		for _, item := range list {
			if s, ok := item.(string); ok {
				set[s] = true
			}
		}
	}
	return set
}

func containsValue(list []any, v any) bool {
	return slices.ContainsFunc(list, func(item any) bool { return reflect.DeepEqual(item, v) })
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}
