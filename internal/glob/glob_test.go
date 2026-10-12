package glob

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"app/**", "app/Http/Kernel.php", true},
		{"app/**", "app", true}, // ** matches zero segments
		{"app/**", "application/x", false},
		{"**/*.php", "app/Http/Kernel.php", true},
		{"**/*.php", "Kernel.php", true},
		{"**/*.php", "app/Http/Kernel.js", false},
		{"app/*/Kernel.php", "app/Http/Kernel.php", true},
		{"app/*/Kernel.php", "app/Http/Sub/Kernel.php", false},
		{"./app/**", "app/x", true},
		{"app/**", "./app//x/", true},
		{"resources/views/admin/**", "resources/views/public/x", false},
		{"**", "anything/at/all", true},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.name); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestBelow(t *testing.T) {
	cases := []struct {
		pattern, dir string
		want         bool
	}{
		{"app/Http/**", "app", true},
		{"app/Http/**", ".", true},
		{"app/Http/**", "app/Models", false},
		{"app/Http/**", "app/Http", true},
		{"**/*.php", "anything", true},
		{"app/*.php", "app", true},
		{"app/*.php", "app/Http", false}, // nothing below app/Http can match app/*.php
	}
	for _, c := range cases {
		if got := Below(c.pattern, c.dir); got != c.want {
			t.Errorf("Below(%q, %q) = %v, want %v", c.pattern, c.dir, got, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	for _, ok := range []string{"app/**", "**/*.php", "src/[a-z]*/x"} {
		if err := Validate(ok); err != nil {
			t.Errorf("Validate(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"/app/**", "../other/**", "app/[unclosed"} {
		if err := Validate(bad); err == nil {
			t.Errorf("Validate(%q) = nil, want an error", bad)
		}
	}
}
