package stack

import (
	"strings"

	"github.com/JustSteveKing/stackfile/internal/glob"
)

// Applies reports whether an entry applies at rel, a slash-separated path
// relative to the stackfile. An entry with no scope applies everywhere. When
// rel is a directory, an entry scoped to anything inside it applies too.
func (e Entry) Applies(rel string, isDir bool) bool {
	if len(e.Scope) == 0 {
		return true
	}
	for _, s := range e.Scope {
		if glob.Match(s, rel) || (isDir && glob.Below(s, rel)) {
			return true
		}
	}
	return false
}

// ForPath returns the entries that apply at rel, in file order.
func (f *File) ForPath(rel string, isDir bool) []Entry {
	var out []Entry
	for _, e := range f.Entries() {
		if e.Applies(rel, isDir) {
			out = append(out, e)
		}
	}
	return out
}

// Verdict is the answer to "may I add this package".
type Verdict string

const (
	// VerdictAllowed: an adopt entry covers it.
	VerdictAllowed Verdict = "allowed"
	// VerdictTrial: a trial entry covers it, so only within its scope.
	VerdictTrial Verdict = "trial"
	// VerdictUndecided: a proposed entry lists it as a candidate, or it is
	// the proposed entry itself. Choosing it is the user's decision.
	VerdictUndecided Verdict = "undecided"
	// VerdictHold: no new uses.
	VerdictHold Verdict = "hold"
	// VerdictRetired: being removed.
	VerdictRetired Verdict = "retired"
	// VerdictAvoid: on the avoid list.
	VerdictAvoid Verdict = "avoid"
	// VerdictUndeclared: the stackfile says nothing either way.
	VerdictUndeclared Verdict = "undeclared"
)

// Answer is a verdict with what it was based on.
type Answer struct {
	Package string  `json:"package"`
	Verdict Verdict `json:"verdict"`
	Entry   *Entry  `json:"entry,omitempty"`
	Avoid   *Avoid  `json:"avoid,omitempty"`
}

// Lookup answers whether pkg may be added. ecosystem narrows the match when
// given; without it, any entry with that package or name counts. A package is
// matched against an entry's package, then its name, so "postgres" finds an
// entry that has no package at all.
func (f *File) Lookup(pkg, ecosystem string) Answer {
	ans := Answer{Package: pkg, Verdict: VerdictUndeclared}
	ecoOK := func(e string) bool { return ecosystem == "" || e == "" || e == ecosystem }

	// The avoid list wins: if something is both a candidate and banned,
	// the ban is the safer thing to tell an agent.
	for i, a := range f.Avoid {
		if ecoOK(a.Ecosystem) && (same(a.Package, pkg) || same(a.Name, pkg)) {
			ans.Verdict, ans.Avoid = VerdictAvoid, &f.Avoid[i]
			return ans
		}
	}

	entries := f.Entries()
	for i, e := range entries {
		if !ecoOK(e.Ecosystem) || !(same(e.Package, pkg) || same(e.Name, pkg)) {
			continue
		}
		ans.Entry = &entries[i]
		switch e.Status {
		case StatusAdopt:
			ans.Verdict = VerdictAllowed
		case StatusTrial:
			ans.Verdict = VerdictTrial
		case StatusProposed:
			ans.Verdict = VerdictUndecided
		case StatusHold:
			ans.Verdict = VerdictHold
		case StatusRetired:
			ans.Verdict = VerdictRetired
		}
		return ans
	}

	for i, e := range entries {
		if e.Status != StatusProposed {
			continue
		}
		for _, c := range e.Candidates {
			if same(c, pkg) {
				ans.Verdict, ans.Entry = VerdictUndecided, &entries[i]
				return ans
			}
		}
	}
	return ans
}

func same(a, b string) bool { return a != "" && strings.EqualFold(a, b) }

// PackageName strips a version from a package as it is typed on an install
// command line: axios@1.7, @types/node@20, laravel/framework:^12.0,
// golang.org/x/text@v0.14.0, requests==2.32.
func PackageName(spec string) string {
	if i := strings.LastIndex(spec, "@"); i > 0 {
		spec = spec[:i]
	}
	if i := strings.IndexAny(spec, ":=<>~!"); i > 0 {
		spec = spec[:i]
	}
	return strings.TrimSpace(spec)
}
