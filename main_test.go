package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "payload"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := run([]string{"--json", dir}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Files   int
		Entries []struct {
			Name     string
			Apparent int64 `json:"apparent_bytes"`
		}
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 || len(result.Entries) != 1 || result.Entries[0].Name != "payload" || result.Entries[0].Apparent != 5 {
		t.Fatalf("unexpected JSON: %s", out.String())
	}
}

func TestCLIArguments(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"a", "b"}, {"--json", filepath.Join(t.TempDir(), "missing")}} {
		var out bytes.Buffer
		if err := run(args, &out, &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, arg := range []string{"--help", "--version"} {
		var out bytes.Buffer
		if err := run([]string{arg}, &out, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "disk-inspect") {
			t.Fatal("missing CLI output")
		}
	}
}
