package deps

import (
	"strings"
	"testing"
)

func readOne(t *testing.T, dir string) *Inventory {
	t.Helper()
	invs, err := Read("testdata/" + dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(invs) != 1 {
		t.Fatalf("got %d inventories, want 1", len(invs))
	}
	return invs[0]
}

type dep struct {
	name, version string
	direct        bool
}

func assertDeps(t *testing.T, inv *Inventory, want []dep) {
	t.Helper()
	got := inv.All()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d dependencies %v, want %d", inv.Ecosystem, len(got), got, len(want))
	}
	for i, w := range want {
		if got[i].Name != w.name || got[i].Version != w.version || got[i].Direct != w.direct {
			t.Errorf("%s: dependency %d = %+v, want %+v", inv.Ecosystem, i, got[i], w)
		}
	}
}

func TestComposer(t *testing.T) {
	inv := readOne(t, "composer")
	if !inv.Locked || strings.Join(inv.Files, ",") != "composer.json,composer.lock" {
		t.Errorf("files %v locked %v", inv.Files, inv.Locked)
	}
	// php and ext-json are platform requirements, not packages.
	assertDeps(t, inv, []dep{
		{"guzzlehttp/guzzle", "7.9.0", false},
		{"laravel/framework", "v13.1.0", true},
		{"phpunit/phpunit", "12.0.3", true},
	})
	if _, ok := inv.Find("Laravel/Framework"); !ok {
		t.Error("composer names match case-insensitively")
	}
}

func TestPackageLock(t *testing.T) {
	inv := readOne(t, "npm-lock")
	// The top-level react wins over the nested copy, whatever order the
	// map is walked in; a package only installed nested still counts.
	assertDeps(t, inv, []dep{
		{"axios", "1.7.0", false},
		{"left-pad", "1.3.0", false},
		{"react", "19.1.0", true},
		{"vitest", "3.0.4", true},
	})
}

func TestPackageLockV1IsANote(t *testing.T) {
	inv := readOne(t, "npm-lock-v1")
	if inv.Locked || len(inv.Notes) != 1 || !strings.Contains(inv.Notes[0], "lockfileVersion 1") {
		t.Errorf("locked %v notes %v", inv.Locked, inv.Notes)
	}
	if d, ok := inv.Find("react"); !ok || !d.Direct || d.Version != "" {
		t.Errorf("react = %+v, %v; want direct with no version", d, ok)
	}
}

func TestBunLock(t *testing.T) {
	inv := readOne(t, "bun")
	if !inv.Locked {
		t.Error("bun.lock should be read")
	}
	assertDeps(t, inv, []dep{
		{"@astrojs/mdx", "7.0.5", true},
		{"react", "19.1.0", true},
		{"string-width", "4.2.3", false},
	})
}

func TestNoLockfile(t *testing.T) {
	inv := readOne(t, "nolock")
	if inv.Locked || len(inv.Notes) != 1 || !strings.Contains(inv.Notes[0], "no lockfile") {
		t.Errorf("locked %v notes %v", inv.Locked, inv.Notes)
	}
}

func TestGoMod(t *testing.T) {
	inv := readOne(t, "gomod")
	assertDeps(t, inv, []dep{
		{"github.com/danielgtaylor/huma/v2", "v2.39.1", true},
		{"github.com/jackc/pgx/v5", "v5.11.0", false},
		{"github.com/spf13/cobra", "v1.10.2", true},
	})
	if d, ok := inv.Module("github.com/danielgtaylor/huma/v2/adapters/humachi"); !ok || d.Name != "github.com/danielgtaylor/huma/v2" {
		t.Errorf("a package path inside a module should find the module, got %+v", d)
	}
	if _, ok := inv.Module("github.com/danielgtaylor/huma"); ok {
		t.Error("v1's path is not v2's module")
	}
	if _, ok := inv.Find("GITHUB.COM/spf13/cobra"); ok {
		t.Error("Go module paths are case-sensitive")
	}
}

func TestNothingToRead(t *testing.T) {
	invs, err := Read(t.TempDir())
	if err != nil || len(invs) != 0 {
		t.Errorf("empty directory: %v, %v", invs, err)
	}
}

func TestStripTrailingCommas(t *testing.T) {
	cases := map[string]string{
		`{"a": [1, 2,],}`:       `{"a": [1, 2]}`,
		`{"a": ",]",}`:          `{"a": ",]"}`,
		`{"a": "x\",}",}`:       `{"a": "x\",}"}`,
		"{\"a\": 1,\n  \n}":     "{\"a\": 1\n  \n}",
		`{"a": [1, 2], "b": 3}`: `{"a": [1, 2], "b": 3}`,
	}
	for in, want := range cases {
		if got := string(stripTrailingCommas([]byte(in))); got != want {
			t.Errorf("stripTrailingCommas(%q) = %q, want %q", in, got, want)
		}
	}
}
