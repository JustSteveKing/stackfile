package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSteveKing/stackfile/stack"
)

func TestSplice(t *testing.T) {
	block := StartMarker + "\nnew\n" + EndMarker + "\n"
	cases := []struct {
		name, doc, want string
	}{
		{"empty file", "", block},
		{"no block yet", "# Title\n\nIntro.\n", "# Title\n\nIntro.\n\n" + block},
		{"replaces in place", "# Title\n\n" + StartMarker + "\nold\n" + EndMarker + "\n\n## After\n",
			"# Title\n\n" + block + "\n## After\n"},
		// An older start marker with different wording is still found.
		{"older marker", "a\n<!-- stackfile:start -->\nold\n" + EndMarker + "\nb\n", "a\n" + block + "b\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Splice(c.doc, block)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got\n%q\nwant\n%q", got, c.want)
			}
		})
	}
	if _, err := Splice("x\n"+StartMarker+"\nno end\n", block); err != ErrUnclosed {
		t.Errorf("unclosed marker: err = %v, want ErrUnclosed", err)
	}
}

// Rendering twice changes nothing, which is what lets --check work.
func TestSpliceIsIdempotent(t *testing.T) {
	block := StartMarker + "\nx\n" + EndMarker + "\n"
	once, _ := Splice("# Doc\n", block)
	twice, _ := Splice(once, block)
	if once != twice {
		t.Errorf("second render changed the file:\n%q\n%q", once, twice)
	}
}

// The repository's AGENTS.md carries a rendered block, and it must match
// stackfile.yml. This is 'stackfile render --check' run on ourselves.
func TestOwnAgentsBlockIsCurrent(t *testing.T) {
	f, _, err := stack.Load(filepath.Join("..", "..", stack.FileName))
	if err != nil || f == nil {
		t.Fatalf("loading stackfile.yml: %v", err)
	}
	doc, err := os.ReadFile(filepath.Join("..", "..", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	next, err := Splice(string(doc), Agents(f))
	if err != nil {
		t.Fatal(err)
	}
	if next != string(doc) {
		t.Error("AGENTS.md is out of date; run 'go run ./cmd/stackfile render'")
	}
}

func TestAgentsSections(t *testing.T) {
	f, ds, err := stack.Load("../../testdata/full.yml")
	if err != nil || f == nil || ds.HasErrors() {
		t.Fatalf("full.yml: %v %v", err, ds)
	}
	got := Agents(f)
	for _, want := range []string{
		"- laravel ^12.0 (backend) in app/**, routes/**",
		"**On trial**, only within the scope given:\n\n- htmx (frontend) in resources/views/admin/**",
		"- search: meilisearch or typesense. Do we need typo tolerance?",
		"- redis (retired): being removed. Use database-queue instead.",
		"- axios: use native fetch instead. One HTTP client is enough. (ADR-0015)",
		"- test: `php artisan test`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("block lacks %q\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, StartMarker) || !strings.HasSuffix(got, EndMarker+"\n") {
		t.Error("block is not wrapped in its markers")
	}
}
