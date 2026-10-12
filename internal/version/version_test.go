package version

import "testing"

func TestSatisfies(t *testing.T) {
	cases := []struct {
		constraint, version string
		tilde               Tilde
		want                bool
	}{
		{"^12.0", "12.4.1", TildeNPM, true},
		{"^12.0", "v12.0.0", TildeNPM, true},
		{"^12.0", "13.0.0", TildeNPM, false},
		{"^12.0", "11.9.9", TildeNPM, false},
		{"^0.3", "0.3.9", TildeNPM, true},
		{"^0.3", "0.4.0", TildeNPM, false},
		{"^0.0.3", "0.0.4", TildeNPM, false},
		{"~1.2.3", "1.2.9", TildeNPM, true},
		{"~1.2.3", "1.3.0", TildeNPM, false},
		{"~1.2", "1.9.0", TildeNPM, false},
		{"~1.2", "1.9.0", TildeComposer, true}, // the one place they differ
		{"~1.2", "2.0.0", TildeComposer, false},
		{"~1", "1.9.0", TildeNPM, true},
		{">= 1.0.2", "v1.0.2", TildeNPM, true},
		{">= 1.0.2", "v1.0.1", TildeNPM, false},
		{">=1.0 <2.0", "1.5.0", TildeNPM, true},
		{">=1.0, <2.0", "2.0.0", TildeNPM, false},
		{"^1.0 || ^2.0", "2.3.0", TildeNPM, true},
		{"^1.0 || ^2.0", "3.0.0", TildeNPM, false},
		{"17", "17.2.0", TildeNPM, true},
		{"17", "18.0.0", TildeNPM, false},
		{"1.27", "1.27.2", TildeNPM, true},
		{"1.2.*", "1.2.7", TildeNPM, true},
		{"1.2.x", "1.3.0", TildeNPM, false},
		{"1.2.3", "1.2.3", TildeNPM, true},
		{"1.2.3", "1.2.4", TildeNPM, false},
		{"=1.2", "1.2.5", TildeNPM, true},
		{"*", "9.9.9", TildeNPM, true},
		{"!=1.2.3", "1.2.3", TildeNPM, false},
		{"^2.0", "2.0.0-rc1", TildeNPM, true},
		{"^1.0", "1.2.3.4", TildeComposer, true},
	}
	for _, c := range cases {
		got, err := Satisfies(c.constraint, c.version, c.tilde)
		if err != nil {
			t.Errorf("Satisfies(%q, %q): %v", c.constraint, c.version, err)
			continue
		}
		if got != c.want {
			t.Errorf("Satisfies(%q, %q, %v) = %v, want %v", c.constraint, c.version, c.tilde, got, c.want)
		}
	}
}

// Anything outside the grammar is an error, never a silent pass.
func TestSatisfiesRejects(t *testing.T) {
	for _, c := range [][2]string{
		{"^12.0", "dev-main"},
		{"1.0 - 2.0", "1.5.0"},
		{"latest", "1.0.0"},
		{"", "1.0.0"},
	} {
		if _, err := Satisfies(c[0], c[1], TildeNPM); err == nil {
			t.Errorf("Satisfies(%q, %q) = nil error", c[0], c[1])
		}
	}
}
