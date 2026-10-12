package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseFile(t *testing.T, path string) (*File, Diagnostics) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return Parse(data)
}

// want is one expected diagnostic: where it is and a fragment of what it says.
type want struct {
	line     int
	severity Severity
	contains string
}

func assertDiagnostics(t *testing.T, ds Diagnostics, wants []want) {
	t.Helper()
	if len(ds) != len(wants) {
		for _, d := range ds {
			t.Log(d)
		}
		t.Fatalf("got %d diagnostics, want %d", len(ds), len(wants))
	}
	for i, w := range wants {
		d := ds[i]
		if d.Line != w.line || d.Severity != w.severity || !strings.Contains(d.Message, w.contains) {
			t.Errorf("diagnostic %d = %s; want line %d %s containing %q", i, d, w.line, w.severity, w.contains)
		}
	}
}

// The repository's own stackfile is the first one anyone reads. It must stay
// clean with --strict, warnings included.
func TestOwnStackfileIsClean(t *testing.T) {
	f, ds := parseFile(t, filepath.Join("..", FileName))
	if f == nil || len(ds) > 0 {
		for _, d := range ds {
			t.Log(d)
		}
		t.Fatal("stackfile.yml at the repository root has diagnostics")
	}
}

func TestFullExampleIsValid(t *testing.T) {
	f, ds := parseFile(t, "../testdata/full.yml")
	if f == nil || len(ds) > 0 {
		for _, d := range ds {
			t.Log(d)
		}
		t.Fatal("testdata/full.yml should be valid")
	}
	if got := len(f.Layers); got != 3 {
		t.Errorf("layers = %d, want 3", got)
	}
	// Order is the author's, not alphabetical.
	if f.Layers[0].Name != "backend" || f.Layers[2].Name != "frontend" {
		t.Errorf("layers out of file order: %v, %v", f.Layers[0].Name, f.Layers[2].Name)
	}
	pg, ok := f.Entry("postgres")
	if !ok {
		t.Fatal("postgres not found")
	}
	if pg.Layer != "data" || pg.Basis != BasisDeclared || pg.Line != 21 {
		t.Errorf("postgres = layer %q basis %q line %d; want data, declared, 21", pg.Layer, pg.Basis, pg.Line)
	}
	if f.Avoid[0].Line != 47 {
		t.Errorf("avoid[0] line = %d, want 47", f.Avoid[0].Line)
	}
	if got := f.Layers[0].Entries[0].Commands; len(got) != 1 || got[0].Name != "test" {
		t.Errorf("laravel commands = %v", got)
	}
}

// Every pass that can run reports in one go: the YAML pass and the schema
// pass both report on broken.yml.
func TestSchemaErrorsAreLocated(t *testing.T) {
	f, ds := parseFile(t, "../testdata/broken.yml")
	if f != nil {
		t.Error("a file that fails the schema must not be returned")
	}
	assertDiagnostics(t, ds, []want{
		{3, SeverityError, "unknown field 'defaults'"},
		{3, SeverityError, "anchors (&d) are not allowed"},
		{8, SeverityError, `version 12.10 is read as a number (12.1); quote it: version: "12.10"`},
		{10, SeverityError, "unknown field 'colour'"},
		{14, SeverityError, "a proposed entry needs candidates"},
		{16, SeverityError, "basis: ratified needs decided_by"},
	})
}

func TestSemanticErrors(t *testing.T) {
	f, ds := parseFile(t, "../testdata/semantic.yml")
	if f == nil {
		t.Fatal("a file that passes the schema is returned, even with semantic errors")
	}
	assertDiagnostics(t, ds, []want{
		{8, SeverityError, "laravel is already declared at layers.backend[0]"},
		{10, SeverityWarning, "symfony is not an entry"},
		{14, SeverityWarning, "set basis: ratified"},
		{15, SeverityError, "drop the leading /"},
		{16, SeverityWarning, "candidates only apply while status is proposed"},
		{18, SeverityError, "both an entry"},
	})
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name, yaml string
		want       want
	}{
		{"empty", "", want{1, SeverityError, "the file is empty"}},
		// yaml.v3 places this error where the flow sequence opens.
		{"syntax", "stackfile: 1\nproject: [unclosed\n", want{1, SeverityError, "did not find expected"}},
		{"two documents", "stackfile: 1\n---\nproject: x\n", want{2, SeverityError, "one YAML document"}},
		{"alias", "stackfile: 1\nproject: &p x\ndescription: *p\nlayers: {a: [{name: a, status: adopt}]}\n",
			want{2, SeverityError, "anchors"}},
		{"integer version", "stackfile: 1\nproject: x\nlayers:\n  a:\n    - name: a\n      status: adopt\n      version: 17\n",
			want{7, SeverityError, `quote it: version: "17"`}},
		{"wrong format version", "stackfile: 2\nproject: x\nlayers: {a: [{name: a, status: adopt}]}\n",
			want{1, SeverityError, "value must be 1"}},
		{"retired without replacement", "stackfile: 1\nproject: x\nlayers:\n  a:\n    - name: a\n      status: retired\n",
			want{5, SeverityError, "a retired entry needs replaced_by"}},
		{"package without ecosystem", "stackfile: 1\nproject: x\nlayers:\n  a:\n    - name: a\n      status: adopt\n      package: a/b\n",
			want{5, SeverityError, "package needs 'ecosystem' too"}},
		{"long rule", "stackfile: 1\nproject: x\nprinciples: [\"" + strings.Repeat("x", 201) + "\"]\nlayers: {a: [{name: a, status: adopt}]}\n",
			want{3, SeverityError, "201 characters, and the limit is 200"}},
		{"self replacement", "stackfile: 1\nproject: x\nlayers:\n  a:\n    - name: a\n      status: retired\n      replaced_by: a\n",
			want{7, SeverityError, "cannot replace itself"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ds := Parse([]byte(c.yaml))
			var found bool
			for _, d := range ds {
				if d.Line == c.want.line && d.Severity == c.want.severity && strings.Contains(d.Message, c.want.contains) {
					found = true
				}
			}
			if !found {
				for _, d := range ds {
					t.Log(d)
				}
				t.Errorf("no %s on line %d containing %q", c.want.severity, c.want.line, c.want.contains)
			}
		})
	}
}

// A float version is only worth a note when YAML changed what was written.
func TestVersionNoteOnlyWhenChanged(t *testing.T) {
	_, ds := Parse([]byte("stackfile: 1\nproject: x\nlayers:\n  a:\n    - name: a\n      status: adopt\n      version: 1.5\n"))
	if len(ds) != 1 || strings.Contains(ds[0].Message, "(") {
		t.Errorf("got %v; want one diagnostic with no '(1.5)' note", ds)
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("stackfile: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Find(deep)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, FileName) {
		t.Errorf("Find = %s", got)
	}

	if _, err := Find(t.TempDir()); err != ErrNotFound {
		// A temp dir's parents could hold a stackfile on some machine, but
		// not on CI or this one; if this fails, check for a stray file.
		t.Errorf("Find in an empty tree = %v, want ErrNotFound", err)
	}
}
