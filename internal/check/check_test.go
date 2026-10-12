package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSteveKing/stackfile/internal/deps"
	"github.com/JustSteveKing/stackfile/stack"
)

func run(t *testing.T, opt Options) Result {
	t.Helper()
	f, ds, err := stack.Load("testdata/project/stackfile.yml")
	if err != nil || f == nil || ds.HasErrors() {
		t.Fatalf("fixture: %v %v", err, ds)
	}
	invs, err := deps.Read(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return Run(f, invs, opt)
}

// Each finding the fixture is built to produce, in the order they are
// reported: errors, warnings, notes, each by stackfile line.
func TestFindings(t *testing.T) {
	res := run(t, Options{})
	want := []struct {
		sev     Severity
		entry   string
		line    int
		message string
	}{
		{SeverityError, "phpunit", 10, "phpunit is declared as ^11.0, but composer.lock has phpunit/phpunit 12.0.3"},
		{SeverityError, "queue", 29, "candidate vitest is already a direct dependency"},
		{SeverityWarning, "cobra", 24, "cobra is retired but github.com/spf13/cobra is still a direct dependency. Finish moving to huma"},
		{SeverityWarning, "weird", 32, "cannot check weird's version"},
		{SeverityWarning, "vue", 38, "vue is declared as adopt, but vue is not in package.json or package-lock.json"},
		// Installed, but only because something else needs it.
		{SeverityWarning, "axios", 47, "'npm explain axios' shows what pulls it in"},
		{SeverityWarning, "guzzle", 52, "'composer why guzzlehttp/guzzle' shows what pulls it in"},
		{SeverityWarning, "left-pad", 56, "'npm explain left-pad'"},
	}
	if len(res.Findings) != len(want) {
		for _, f := range res.Findings {
			t.Logf("%s %s:%d %s", f.Severity, f.Entry, f.Line, f.Message)
		}
		t.Fatalf("got %d findings, want %d", len(res.Findings), len(want))
	}
	for i, w := range want {
		f := res.Findings[i]
		if f.Severity != w.sev || f.Entry != w.entry || f.Line != w.line || !strings.Contains(f.Message, w.message) {
			t.Errorf("finding %d = %s %s:%d %q; want %s %s:%d containing %q", i, f.Severity, f.Entry, f.Line, f.Message, w.sev, w.entry, w.line, w.message)
		}
	}
}

// laravel ^13.0 is met by v13.1.0; huma is found through a package path
// inside its module; pgx is only indirect, which still counts as installed.
func TestWhatPasses(t *testing.T) {
	for _, f := range run(t, Options{}).Findings {
		switch f.Entry {
		case "laravel", "huma", "pgx":
			t.Errorf("%s should pass: %s", f.Entry, f.Message)
		}
	}
}

func TestDirectAvoidIsAnError(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", `{"dependencies": {"axios": "^1.7.0"}}`)
	write(stack.FileName, "stackfile: 1\nproject: x\nlayers: {a: [{name: a, status: adopt}]}\navoid:\n  - name: axios\n    ecosystem: npm\n    package: axios\n    use_instead: native fetch\n    why: One client.\n")
	f, _, err := stack.Load(filepath.Join(dir, stack.FileName))
	if err != nil || f == nil {
		t.Fatal(err)
	}
	invs, err := deps.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(f, invs, Options{})
	if len(res.Findings) != 1 || res.Findings[0].Severity != SeverityError ||
		!strings.Contains(res.Findings[0].Message, "axios is a direct dependency, but it is on the avoid list. Use native fetch instead.") {
		t.Errorf("findings = %+v", res.Findings)
	}
}

func TestSkipped(t *testing.T) {
	res := run(t, Options{})
	if len(res.Skipped) != 1 || res.Skipped[0].Ecosystem != "cargo" || res.Skipped[0].Reason != "not supported yet" {
		t.Errorf("skipped = %+v", res.Skipped)
	}
}

func TestUndeclared(t *testing.T) {
	res := run(t, Options{Undeclared: true})
	var notes []string
	for _, f := range res.Findings {
		if f.Severity == SeverityNote {
			notes = append(notes, f.Package)
		}
	}
	// Direct dependencies no entry, candidate or avoid item accounts for.
	if strings.Join(notes, ",") != "zod" {
		t.Errorf("undeclared = %v", notes)
	}
}
