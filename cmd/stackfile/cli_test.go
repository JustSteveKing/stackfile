package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run executes the CLI in-process and returns its output and exit code, as
// main would compute it.
func run(t *testing.T, args ...string) (string, int) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	var code exitCode
	switch {
	case err == nil:
		return out.String(), exitOK
	case errors.As(err, &code):
		return out.String(), int(code)
	default:
		return out.String() + err.Error(), exitError
	}
}

const full = "../../testdata/full.yml"

// Hooks decide from the exit code alone, so each verdict's code is part of
// the contract.
func TestCanIAddExitCodes(t *testing.T) {
	cases := []struct {
		pkg  string
		code int
		says string
	}{
		{"laravel/framework:^12.0", exitOK, "Allowed:"},
		{"htmx.org", exitOK, "Allowed on trial"},
		{"axios@1.7", exitGate, "Use native fetch instead"},
		{"predis/predis", exitGate, "retired"},
		{"meilisearch", exitGate, "Stop and ask the user"},
		{"lodash", exitUndeclared, "Undeclared"},
	}
	for _, c := range cases {
		out, code := run(t, "-f", full, "can-i", "add", c.pkg)
		if code != c.code || !strings.Contains(out, c.says) {
			t.Errorf("can-i add %s: exit %d, want %d; output:\n%s", c.pkg, code, c.code, out)
		}
	}
}

func TestCanIAddJSON(t *testing.T) {
	out, code := run(t, "-f", full, "can-i", "add", "axios", "--format", "json")
	if code != exitGate {
		t.Fatalf("exit %d", code)
	}
	var ans struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(out), &ans); err != nil || ans.Verdict != "avoid" {
		t.Errorf("verdict %q, err %v; output:\n%s", ans.Verdict, err, out)
	}
}

func TestValidateExitCodes(t *testing.T) {
	if out, code := run(t, "validate", full); code != exitOK || !strings.Contains(out, "valid") {
		t.Errorf("full.yml: exit %d\n%s", code, out)
	}
	if out, code := run(t, "validate", "../../testdata/broken.yml"); code != exitGate || !strings.Contains(out, "broken.yml:8:16: error:") {
		t.Errorf("broken.yml: exit %d\n%s", code, out)
	}
	if _, code := run(t, "validate", "../../testdata/does-not-exist.yml"); code != exitError {
		t.Errorf("missing file: exit %d, want %d", code, exitError)
	}
}

// Warnings pass unless --strict.
func TestValidateStrict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stackfile.yml")
	yaml := "stackfile: 1\nproject: x\nlayers:\n  a:\n    - name: a\n      status: adopt\n      decided_by: [ADR-1]\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := run(t, "validate", path); code != exitOK {
		t.Errorf("warnings only: exit %d, want 0", code)
	}
	if _, code := run(t, "validate", "--strict", path); code != exitGate {
		t.Errorf("warnings with --strict: exit %d, want 1", code)
	}
}

// A broken stackfile is refused by every command that acts on it.
func TestCommandsRefuseABrokenFile(t *testing.T) {
	for _, args := range [][]string{{"explain"}, {"can-i", "add", "x"}, {"render", "--stdout"}} {
		out, code := run(t, append([]string{"-f", "../../testdata/semantic.yml"}, args...)...)
		if code != exitError || !strings.Contains(out, "run 'stackfile validate'") {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
}

func TestRenderWritesAndChecks(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	sf := filepath.Join(dir, "stackfile.yml")
	if err := os.WriteFile(sf, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := run(t, "-f", sf, "render", "--check"); code != exitGate {
		t.Errorf("--check before render: exit %d, want 1", code)
	}
	if out, code := run(t, "-f", sf, "render"); code != exitOK || !strings.Contains(out, "wrote") {
		t.Errorf("render: exit %d\n%s", code, out)
	}
	if _, code := run(t, "-f", sf, "render", "--check"); code != exitOK {
		t.Errorf("--check after render: exit %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not created next to the stackfile: %v", err)
	}
}

func TestExplainOutsideProject(t *testing.T) {
	out, code := run(t, "-f", full, "explain", "/")
	if code != exitError || !strings.Contains(out, "outside the project") {
		t.Errorf("exit %d\n%s", code, out)
	}
}
