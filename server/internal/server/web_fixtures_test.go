package server_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the /web/api contract fixtures in "+fixtureDir)

// fixtureDir holds the GUI backend's wire fixtures; gui/tests/api/contract.test.ts decodes them.
const fixtureDir = "../web/testdata"

// volatile maps the keys whose string values differ per run to a fixed placeholder; "id" is
// only volatile as text (an invite id), numeric ids are stable in a fresh database.
var volatile = map[string]string{
	"createdAt": "2026-11-04T20:00:00Z", "lastPostAt": "2026-11-04T20:00:00Z",
	"expiresAt": "2026-11-04T20:10:00Z", "expires_at": "2026-11-04T20:10:00Z", "offlineSince": "2026-11-04T19:00:00Z",
	"csrf": "csrf-of-the-session", "token": "token-shown-once", "code": "code-shown-once",
	"id": "0123456789abcdef",
}

// normalized returns the JSON body raw with its volatile values replaced, indented.
func normalized(t *testing.T, raw string) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("fixture body %s: %v", raw, err)
	}
	out, err := json.MarshalIndent(scrub(v), "", "\t")
	if err != nil {
		t.Fatalf("encoding fixture: %v", err)
	}
	return append(out, '\n')
}

// scrub replaces the volatile string values in v, recursively.
func scrub(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, isText := val.(string); isText && volatile[k] != "" && s != "" {
				x[k] = volatile[k]
			} else {
				x[k] = scrub(val)
			}
		}
	case []any:
		for i, val := range x {
			x[i] = scrub(val)
		}
	}
	return v
}

// checkFixtures compares got (file name → body) with fixtureDir, or rewrites it with -update.
func checkFixtures(t *testing.T, got map[string][]byte) {
	t.Helper()
	if *update {
		writeFixtures(t, got)
		return
	}
	for name, body := range got {
		want, err := os.ReadFile(filepath.Join(fixtureDir, name))
		if err != nil || !bytes.Equal(want, body) {
			t.Errorf("%s is stale (%v); run: go test ./internal/server -run TestWebContractFixtures -update\ngot:\n%s", name, err, body)
		}
	}
	if files := fixtureFiles(t); !slices.Equal(files, sortedNames(got)) {
		t.Errorf("%s holds %v, the test writes %v", fixtureDir, files, sortedNames(got))
	}
}

func writeFixtures(t *testing.T, got map[string][]byte) {
	t.Helper()
	if err := os.RemoveAll(fixtureDir); err != nil {
		t.Fatalf("clearing %s: %v", fixtureDir, err)
	}
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", fixtureDir, err)
	}
	for name, body := range got {
		if err := os.WriteFile(filepath.Join(fixtureDir, name), body, 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
}

// fixtureFiles lists the file names in fixtureDir, sorted; a missing directory is empty.
func fixtureFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(fixtureDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("listing %s: %v", fixtureDir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func sortedNames(got map[string][]byte) []string {
	names := make([]string, 0, len(got))
	for name := range got {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
