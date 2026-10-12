package stack

import "testing"

func fullExample(t *testing.T) *File {
	t.Helper()
	f, ds := parseFile(t, "../testdata/full.yml")
	if f == nil || ds.HasErrors() {
		t.Fatalf("testdata/full.yml: %v", ds)
	}
	return f
}

func TestLookup(t *testing.T) {
	f := fullExample(t)
	cases := []struct {
		pkg, ecosystem string
		want           Verdict
		entry          string
	}{
		{"laravel/framework", "", VerdictAllowed, "laravel"},
		{"laravel/framework", "composer", VerdictAllowed, "laravel"},
		{"Laravel/Framework", "", VerdictAllowed, "laravel"},
		{"laravel/framework", "npm", VerdictUndeclared, ""},
		{"postgres", "", VerdictAllowed, "postgres"}, // by name: no package declared
		{"htmx.org", "npm", VerdictTrial, "htmx"},
		{"predis/predis", "composer", VerdictRetired, "redis"},
		{"search", "", VerdictUndecided, "search"},
		{"typesense", "", VerdictUndecided, "search"}, // a candidate
		{"axios", "npm", VerdictAvoid, ""},
		{"axios", "", VerdictAvoid, ""},
		{"lodash", "npm", VerdictUndeclared, ""},
	}
	for _, c := range cases {
		got := f.Lookup(c.pkg, c.ecosystem)
		if got.Verdict != c.want {
			t.Errorf("Lookup(%q, %q) = %s, want %s", c.pkg, c.ecosystem, got.Verdict, c.want)
			continue
		}
		if c.entry != "" && (got.Entry == nil || got.Entry.Name != c.entry) {
			t.Errorf("Lookup(%q) entry = %v, want %s", c.pkg, got.Entry, c.entry)
		}
	}
}

func TestForPath(t *testing.T) {
	f := fullExample(t)
	names := func(es []Entry) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.Name)
		}
		return out
	}
	cases := []struct {
		rel   string
		isDir bool
		want  []string
	}{
		// Unscoped entries apply everywhere.
		{"database/migrations/x.php", false, []string{"postgres", "database-queue", "redis", "search"}},
		{"app/Http/Kernel.php", false, []string{"laravel", "postgres", "database-queue", "redis", "search"}},
		{"resources/views/admin/x.blade.php", false, []string{"postgres", "database-queue", "redis", "search", "htmx"}},
		// A directory picks up entries scoped to anything inside it.
		{"resources", true, []string{"postgres", "database-queue", "redis", "search", "htmx"}},
		{"resources/views/public", true, []string{"postgres", "database-queue", "redis", "search"}},
		{".", true, []string{"laravel", "postgres", "database-queue", "redis", "search", "htmx"}},
	}
	for _, c := range cases {
		got := names(f.ForPath(c.rel, c.isDir))
		if len(got) != len(c.want) {
			t.Errorf("ForPath(%q) = %v, want %v", c.rel, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ForPath(%q) = %v, want %v", c.rel, got, c.want)
				break
			}
		}
	}
}

func TestPackageName(t *testing.T) {
	cases := map[string]string{
		"axios":                     "axios",
		"axios@1.7":                 "axios",
		"@types/node":               "@types/node",
		"@types/node@20":            "@types/node",
		"laravel/framework:^12.0":   "laravel/framework",
		"golang.org/x/text@v0.14.0": "golang.org/x/text",
		"requests==2.32":            "requests",
		"requests>=2":               "requests",
		"serde@1":                   "serde",
	}
	for in, want := range cases {
		if got := PackageName(in); got != want {
			t.Errorf("PackageName(%q) = %q, want %q", in, got, want)
		}
	}
}
