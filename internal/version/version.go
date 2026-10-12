// Package version checks an installed version against the constraint a
// stackfile declares. It covers what people write in a stackfile, not every
// corner of every ecosystem's grammar: ^ and ~, the comparison operators,
// wildcards, bare prefixes, AND by comma or space and OR by ||. Anything else
// is an error, which check reports as "cannot check" rather than guessing.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// V is a version reduced to three numbers. Pre-release and build suffixes
// are dropped: a stackfile constrains releases, and 2.0.0-rc1 satisfying
// ^2.0 is the answer a person would give.
type V [3]int

// Parse reads 1, 1.2, 1.2.3, v1.2.3, 1.2.3-rc1+build. It reports how many
// parts were given, which ^, ~ and bare prefixes depend on.
func Parse(s string) (V, int, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return V{}, 0, fmt.Errorf("empty version")
	}
	var v V
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		// 1.2.3.4 appears in composer for some packages; the fourth part is
		// below anything a stackfile constrains.
		parts = parts[:3]
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return V{}, 0, fmt.Errorf("%q is not a version", s)
		}
		v[i] = n
	}
	return v, len(parts), nil
}

func (v V) cmp(o V) int {
	for i := range v {
		if v[i] != o[i] {
			if v[i] < o[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func (v V) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

// bump returns the smallest version above every version that starts with
// the first n parts of v: bump(1.2.3, 2) is 1.3.0.
func bump(v V, n int) V {
	var out V
	copy(out[:n], v[:n])
	out[n-1]++
	return out
}

// Tilde says how ~ reads a constraint with two parts. npm and Go tools read
// ~1.2 as >=1.2 <1.3; composer reads it as >=1.2 <2.0, bumping the
// second-to-last part given. They agree when three parts are given.
type Tilde int

const (
	TildeNPM Tilde = iota
	TildeComposer
)

type bound struct {
	op string // >=, >, <=, <, =, !=
	v  V
}

// Satisfies reports whether version meets constraint.
func Satisfies(constraint, version string, tilde Tilde) (bool, error) {
	got, _, err := Parse(version)
	if err != nil {
		return false, err
	}
	alternatives := strings.Split(constraint, "||")
	for _, alt := range alternatives {
		bounds, err := parseAll(alt, tilde)
		if err != nil {
			return false, err
		}
		ok := true
		for _, b := range bounds {
			if !b.holds(got) {
				ok = false
				break
			}
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func (b bound) holds(v V) bool {
	c := v.cmp(b.v)
	switch b.op {
	case ">=":
		return c >= 0
	case ">":
		return c > 0
	case "<=":
		return c <= 0
	case "<":
		return c < 0
	case "!=":
		return c != 0
	}
	return c == 0
}

// parseAll reads one AND group. Operators may be separated from their
// version by a space (">= 1.0.2"), so a lone operator joins the next token.
func parseAll(s string, tilde Tilde) ([]bound, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty constraint")
	}
	var bounds []bound
	for i := 0; i < len(fields); i++ {
		term := fields[i]
		if strings.Trim(term, "<>=!^~") == "" && i+1 < len(fields) {
			term += fields[i+1]
			i++
		}
		if term == "-" {
			return nil, fmt.Errorf("hyphen ranges are not supported; write >= and <")
		}
		bs, err := parseTerm(term, tilde)
		if err != nil {
			return nil, err
		}
		bounds = append(bounds, bs...)
	}
	return bounds, nil
}

func parseTerm(t string, tilde Tilde) ([]bound, error) {
	if t == "*" || t == "x" || t == "X" {
		return nil, nil
	}
	for _, op := range []string{">=", "<=", "!=", "==", ">", "<", "=", "^", "~"} {
		if !strings.HasPrefix(t, op) {
			continue
		}
		rest := strings.TrimSpace(t[len(op):])
		v, n, err := Parse(rest)
		if err != nil {
			return nil, err
		}
		switch op {
		case "^":
			// The first non-zero part given is the one that may not change.
			i := 0
			for i < n-1 && v[i] == 0 {
				i++
			}
			return []bound{{">=", v}, {"<", bump(v, i+1)}}, nil
		case "~":
			// ~1 and ~1.2.3 read the same everywhere: <2 and <1.3.
			// Only ~1.2 differs: <1.3 for npm, <2.0 for composer.
			k := 2
			if n == 1 || (n == 2 && tilde == TildeComposer) {
				k = 1
			}
			return []bound{{">=", v}, {"<", bump(v, k)}}, nil
		case "==":
			op = "="
		}
		if op == "=" {
			return prefix(v, n), nil
		}
		return []bound{{op, v}}, nil
	}
	// A bare version or wildcard: 1.2, 1.2.*, 1.2.x, 17.
	trimmed := strings.TrimRight(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(t, ".*"), ".x"), ".X"), ".")
	v, n, err := Parse(trimmed)
	if err != nil {
		return nil, err
	}
	return prefix(v, n), nil
}

// prefix matches every version that starts with the parts given: 1.2 is
// >=1.2.0 <1.3.0, and 1.2.3 is exactly 1.2.3.
func prefix(v V, n int) []bound {
	if n == 3 {
		return []bound{{"=", v}}
	}
	return []bound{{">=", v}, {"<", bump(v, n)}}
}
