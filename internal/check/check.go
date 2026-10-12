// Package check compares a stackfile with what the project actually
// installs. A stackfile nobody checks drifts into fiction, and an agent
// trusting fiction is worse off than one with no stackfile at all.
package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/JustSteveKing/stackfile/internal/deps"
	"github.com/JustSteveKing/stackfile/internal/version"
	"github.com/JustSteveKing/stackfile/stack"
)

// Severity of a finding. Errors are where the stackfile and the project
// contradict each other; warnings are expected states worth a reminder;
// notes are information that never fails a run.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityNote    Severity = "note"
)

// Finding is one disagreement between the stackfile and the project.
type Finding struct {
	Severity  Severity `json:"severity"`
	Ecosystem string   `json:"ecosystem"`
	Package   string   `json:"package"`
	// Entry is the stackfile entry or avoid item involved, and Line where it
	// is. Both are empty for an undeclared dependency.
	Entry string `json:"entry,omitempty"`
	Line  int    `json:"line,omitempty"`
	// File is the manifest or lockfile the evidence came from.
	File    string `json:"file"`
	Message string `json:"message"`
}

// Skipped is an ecosystem the stackfile uses that could not be checked.
type Skipped struct {
	Ecosystem string `json:"ecosystem"`
	Reason    string `json:"reason"`
}

// Result is everything check found.
type Result struct {
	Checked  []*deps.Inventory `json:"checked"`
	Skipped  []Skipped         `json:"skipped"`
	Findings []Finding         `json:"findings"`
}

// Count returns the number of findings at a severity.
func (r Result) Count(s Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// Options change what is reported.
type Options struct {
	// Undeclared adds a note for every direct dependency the stackfile
	// does not mention. Off by default: undeclared is not forbidden.
	Undeclared bool
}

// manifests names the file whose absence means an ecosystem is not in use.
var manifests = map[string]string{"composer": "composer.json", "npm": "package.json", "go": "go.mod"}

type checker struct {
	f   *stack.File
	inv map[string]*deps.Inventory
	// used records each dependency some entry or avoid item accounts for,
	// keyed by ecosystem and name, so the rest can be reported undeclared.
	used map[string]bool
	out  []Finding
}

// Run compares f with the inventories read from the project.
func Run(f *stack.File, invs []*deps.Inventory, opt Options) Result {
	c := &checker{f: f, inv: map[string]*deps.Inventory{}, used: map[string]bool{}}
	for _, inv := range invs {
		c.inv[inv.Ecosystem] = inv
	}

	res := Result{Checked: invs, Skipped: []Skipped{}}
	if res.Checked == nil {
		res.Checked = []*deps.Inventory{}
	}
	seen := map[string]bool{}
	skip := func(eco string) {
		if eco == "" || c.inv[eco] != nil || seen[eco] {
			return
		}
		seen[eco] = true
		if m, ok := manifests[eco]; ok {
			res.Skipped = append(res.Skipped, Skipped{eco, "no " + m})
		} else {
			res.Skipped = append(res.Skipped, Skipped{eco, "not supported yet"})
		}
	}

	for _, e := range f.Entries() {
		skip(e.Ecosystem)
		c.entry(e)
	}
	for _, a := range f.Avoid {
		skip(a.Ecosystem)
		c.avoid(a)
	}
	if opt.Undeclared {
		c.undeclared()
	}

	order := map[Severity]int{SeverityError: 0, SeverityWarning: 1, SeverityNote: 2}
	sort.SliceStable(c.out, func(i, j int) bool {
		if order[c.out[i].Severity] != order[c.out[j].Severity] {
			return order[c.out[i].Severity] < order[c.out[j].Severity]
		}
		return c.out[i].Line < c.out[j].Line
	})
	res.Findings = c.out
	if res.Findings == nil {
		res.Findings = []Finding{}
	}
	return res
}

// find looks a package up in its ecosystem. A Go entry may name a package
// inside a module; it is matched to the module.
func (c *checker) find(eco, pkg string) (deps.Dependency, *deps.Inventory, bool) {
	inv := c.inv[eco]
	if inv == nil {
		return deps.Dependency{}, nil, false
	}
	var d deps.Dependency
	var ok bool
	if eco == "go" {
		d, ok = inv.Module(pkg)
	} else {
		d, ok = inv.Find(pkg)
	}
	if ok {
		c.used[eco+"\x00"+strings.ToLower(d.Name)] = true
	}
	return d, inv, ok
}

func (c *checker) add(sev Severity, eco, pkg, entry string, line int, inv *deps.Inventory, format string, args ...any) {
	c.out = append(c.out, Finding{
		Severity: sev, Ecosystem: eco, Package: pkg, Entry: entry, Line: line,
		File: evidence(inv), Message: fmt.Sprintf(format, args...),
	})
}

func (c *checker) entry(e stack.Entry) {
	// A proposed entry's candidates are checked whether or not the entry
	// names a package: installing one is choosing it.
	if e.Status == stack.StatusProposed {
		for _, cand := range e.Candidates {
			for _, eco := range c.ecosystems(e.Ecosystem) {
				if d, inv, ok := c.find(eco, cand); ok && d.Direct {
					c.add(SeverityError, eco, d.Name, e.Name, e.Line, inv,
						"%s is undecided in %s, but candidate %s is already a direct dependency. Something chose it without recording the decision: ask the user, then set %s to adopt with this package, or remove the package.",
						e.Name, stack.FileName, d.Name, e.Name)
				}
			}
		}
	}
	if e.Ecosystem == "" || e.Package == "" {
		return
	}
	d, inv, ok := c.find(e.Ecosystem, e.Package)
	if inv == nil {
		return
	}

	switch e.Status {
	case stack.StatusAdopt, stack.StatusTrial:
		if !ok {
			c.add(SeverityWarning, e.Ecosystem, e.Package, e.Name, e.Line, inv,
				"%s is declared as %s, but %s is not in %s. Install it if the project should use it, or remove the entry if it no longer does.",
				e.Name, e.Status, e.Package, strings.Join(inv.Files, " or "))
			return
		}
		c.version(e, d, inv)
	case stack.StatusProposed:
		if ok && d.Direct {
			c.add(SeverityError, e.Ecosystem, d.Name, e.Name, e.Line, inv,
				"%s is undecided in %s, but %s is already a direct dependency. Ask the user, then set %s to adopt, or remove the package.",
				e.Name, stack.FileName, d.Name, e.Name)
		}
	case stack.StatusRetired:
		if ok && d.Direct {
			c.add(SeverityWarning, e.Ecosystem, d.Name, e.Name, e.Line, inv,
				"%s is retired but %s is still a direct dependency. Finish moving to %s and remove it, or set %s to hold if it is staying.",
				e.Name, d.Name, e.ReplacedBy, e.Name)
		}
	case stack.StatusHold:
		// Existing uses may stay; only new ones are forbidden, and a
		// lockfile cannot tell old from new.
	}
}

func (c *checker) version(e stack.Entry, d deps.Dependency, inv *deps.Inventory) {
	if e.Version == "" || d.Version == "" {
		return
	}
	tilde := version.TildeNPM
	if e.Ecosystem == "composer" {
		tilde = version.TildeComposer
	}
	ok, err := version.Satisfies(e.Version, d.Version, tilde)
	if err != nil {
		c.add(SeverityWarning, e.Ecosystem, d.Name, e.Name, e.Line, inv,
			"cannot check %s's version %q against %s: %v", e.Name, e.Version, d.Version, err)
		return
	}
	if !ok {
		c.add(SeverityError, e.Ecosystem, d.Name, e.Name, e.Line, inv,
			"%s is declared as %s, but %s has %s %s. Change one to match the other; if the declared version is a decision, ask the user before changing it.",
			e.Name, e.Version, evidence(inv), d.Name, d.Version)
	}
}

func (c *checker) avoid(a stack.Avoid) {
	if a.Ecosystem == "" || a.Package == "" {
		return
	}
	d, inv, ok := c.find(a.Ecosystem, a.Package)
	if !ok {
		return
	}
	instead := ""
	if a.UseInstead != "" {
		instead = " Use " + a.UseInstead + " instead."
	}
	if d.Direct {
		c.add(SeverityError, a.Ecosystem, d.Name, a.Name, a.Line, inv,
			"%s is a direct dependency, but it is on the avoid list.%s Remove it, or, if it is needed, stop and ask the user to change %s.",
			d.Name, instead, stack.FileName)
		return
	}
	c.add(SeverityWarning, a.Ecosystem, d.Name, a.Name, a.Line, inv,
		"%s %s is installed because another package depends on it, and it is on the avoid list. Nothing here asks for it directly; '%s' shows what pulls it in.",
		d.Name, d.Version, whyCommand(inv, d.Name))
}

func (c *checker) undeclared() {
	for _, eco := range c.ecosystems("") {
		inv := c.inv[eco]
		for _, d := range inv.All() {
			if !d.Direct || c.used[eco+"\x00"+strings.ToLower(d.Name)] {
				continue
			}
			v := ""
			if d.Version != "" {
				v = " " + d.Version
			}
			c.out = append(c.out, Finding{Severity: SeverityNote, Ecosystem: eco, Package: d.Name, File: evidence(inv),
				Message: fmt.Sprintf("%s%s is a direct dependency that %s does not mention. Declare it if choosing it was a decision.", d.Name, v, stack.FileName)})
		}
	}
}

// ecosystems returns eco when given, or every ecosystem read, in a stable
// order.
func (c *checker) ecosystems(eco string) []string {
	if eco != "" {
		if c.inv[eco] == nil {
			return nil
		}
		return []string{eco}
	}
	var out []string
	for e := range c.inv {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// evidence names the file versions came from: the lockfile when there is
// one, else the manifest.
func evidence(inv *deps.Inventory) string {
	if inv == nil || len(inv.Files) == 0 {
		return ""
	}
	return inv.Files[len(inv.Files)-1]
}

func whyCommand(inv *deps.Inventory, name string) string {
	switch inv.Ecosystem {
	case "composer":
		return "composer why " + name
	case "go":
		return "go mod why -m " + name
	}
	for _, f := range inv.Files {
		if f == "bun.lock" {
			return "bun why " + name
		}
	}
	return "npm explain " + name
}
