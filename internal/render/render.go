// Package render writes what stackfile tells agents and people. Most of its
// output is read by a coding agent deciding what to do next, so every message
// says what to do instead and when to stop and ask, not only what is wrong.
package render

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/JustSteveKing/stackfile/stack"
)

// Explain writes what applies at rel: the principles, the entries in scope
// grouped by layer, what is undecided, what not to add, and the commands.
func Explain(w io.Writer, f *stack.File, rel string, isDir bool) {
	fmt.Fprintf(w, "%s, from %s\n", f.Project, filepath.Base(f.Path))
	if rel != "" && rel != "." {
		fmt.Fprintf(w, "Applies to: %s\n", rel)
	}

	entries := f.ForPath(rel, isDir)
	var use, undecided, stop []stack.Entry
	for _, e := range entries {
		switch e.Status {
		case stack.StatusAdopt, stack.StatusTrial:
			use = append(use, e)
		case stack.StatusProposed:
			undecided = append(undecided, e)
		default:
			stop = append(stop, e)
		}
	}

	if len(f.Principles) > 0 {
		fmt.Fprintln(w, "\nPrinciples (tie-breakers for anything the entries below do not cover)")
		for _, p := range f.Principles {
			fmt.Fprintf(w, "  - %s\n", p)
		}
	}

	if len(use) > 0 {
		layer := ""
		for _, e := range use {
			if e.Layer != layer {
				layer = e.Layer
				fmt.Fprintf(w, "\n%s\n", layer)
			}
			fmt.Fprintf(w, "  %s\n", headline(e))
			if e.Status == stack.StatusTrial {
				fmt.Fprintf(w, "    On trial: use it only within %s, and do not spread it further.\n", scopeText(e))
			}
			if e.Why != "" {
				fmt.Fprintf(w, "    Why: %s\n", e.Why)
			}
			for _, r := range e.Rules {
				fmt.Fprintf(w, "    - %s\n", r)
			}
			for _, c := range e.Commands {
				fmt.Fprintf(w, "    %s: %s\n", c.Name, c.Run)
			}
			if e.Docs != "" {
				fmt.Fprintf(w, "    Docs: %s\n", e.Docs)
			}
			if len(e.DecidedBy) > 0 {
				fmt.Fprintf(w, "    Decided by: %s\n", strings.Join(e.DecidedBy, ", "))
			}
		}
	}

	if len(undecided) > 0 {
		fmt.Fprintln(w, "\nUndecided: ask the user before choosing any of these")
		for _, e := range undecided {
			fmt.Fprintf(w, "  %s: %s\n", e.Name, strings.Join(e.Candidates, " or "))
			if e.Open != "" {
				fmt.Fprintf(w, "    Open question: %s\n", e.Open)
			}
		}
	}

	if len(stop) > 0 || len(f.Avoid) > 0 {
		fmt.Fprintln(w, "\nDo not add")
		for _, e := range stop {
			fmt.Fprintf(w, "  %s\n", stopLine(e))
		}
		for _, a := range f.Avoid {
			fmt.Fprintf(w, "  %s\n", avoidLine(a))
		}
	}

	if len(f.Commands) > 0 {
		fmt.Fprintln(w, "\nCommands")
		for _, c := range f.Commands {
			fmt.Fprintf(w, "  %s: %s\n", c.Name, c.Run)
		}
	}
}

// ExplainData is Explain's content for --format json.
type ExplainData struct {
	Project    string          `json:"project"`
	Path       string          `json:"path"`
	Principles []string        `json:"principles"`
	Entries    []stack.Entry   `json:"entries"`
	Avoid      []stack.Avoid   `json:"avoid"`
	Commands   []stack.Command `json:"commands"`
}

// ExplainJSON returns what Explain writes, as data.
func ExplainJSON(f *stack.File, rel string, isDir bool) ExplainData {
	d := ExplainData{
		Project:    f.Project,
		Path:       rel,
		Principles: f.Principles,
		Entries:    f.ForPath(rel, isDir),
		Avoid:      f.Avoid,
		Commands:   f.Commands,
	}
	// Empty lists rather than null, so a consumer can range without a check.
	if d.Principles == nil {
		d.Principles = []string{}
	}
	if d.Entries == nil {
		d.Entries = []stack.Entry{}
	}
	if d.Avoid == nil {
		d.Avoid = []stack.Avoid{}
	}
	if d.Commands == nil {
		d.Commands = []stack.Command{}
	}
	return d
}

// Answer writes the reply to "may I add this package". The first line is
// the verdict; the rest is what to do about it.
func Answer(w io.Writer, a stack.Answer) {
	e, av := a.Entry, a.Avoid
	switch a.Verdict {
	case stack.VerdictAllowed:
		fmt.Fprintf(w, "Allowed: %s is adopted (%s).\n", a.Package, e.Name)
		if len(e.Scope) > 0 {
			fmt.Fprintf(w, "It applies within %s.\n", scopeText(*e))
		}
		for _, r := range e.Rules {
			fmt.Fprintf(w, "- %s\n", r)
		}
	case stack.VerdictTrial:
		fmt.Fprintf(w, "Allowed on trial: %s may be used only within %s.\n", a.Package, scopeText(*e))
		fmt.Fprintln(w, "Do not use it outside that scope. If you need it elsewhere, ask the user.")
		for _, r := range e.Rules {
			fmt.Fprintf(w, "- %s\n", r)
		}
	case stack.VerdictUndecided:
		fmt.Fprintf(w, "Undecided: %s has not been chosen yet", e.Name)
		if len(e.Candidates) > 0 {
			fmt.Fprintf(w, " (candidates: %s)", strings.Join(e.Candidates, ", "))
		}
		fmt.Fprintln(w, ".")
		if e.Open != "" {
			fmt.Fprintf(w, "Open question: %s\n", e.Open)
		}
		fmt.Fprintf(w, "Do not add %s. Stop and ask the user to decide; this is their call, not yours.\n", a.Package)
	case stack.VerdictHold:
		fmt.Fprintf(w, "Not allowed: %s is on hold. Existing uses may stay, but do not add new ones.\n", a.Package)
		if e.ReplacedBy != "" {
			fmt.Fprintf(w, "Use %s instead.\n", e.ReplacedBy)
		}
		reason(w, e.Why, e.DecidedBy)
		tellUser(w)
	case stack.VerdictRetired:
		fmt.Fprintf(w, "Not allowed: %s is retired and being removed. Use %s instead.\n", a.Package, e.ReplacedBy)
		reason(w, e.Why, e.DecidedBy)
		tellUser(w)
	case stack.VerdictAvoid:
		fmt.Fprintf(w, "Not allowed: %s is on the avoid list.", a.Package)
		if av.UseInstead != "" {
			fmt.Fprintf(w, " Use %s instead.", av.UseInstead)
		}
		fmt.Fprintln(w)
		reason(w, av.Why, av.DecidedBy)
		tellUser(w)
	case stack.VerdictUndeclared:
		fmt.Fprintf(w, "Undeclared: %s is not in %s, which neither allows nor forbids it.\n", a.Package, stack.FileName)
		fmt.Fprintln(w, "Check whether something already in the stack covers the need (run 'stackfile explain').")
		fmt.Fprintf(w, "If you add it, tell the user, so they can declare it in %s.\n", stack.FileName)
	}
}

func reason(w io.Writer, why string, decidedBy []string) {
	if why != "" {
		fmt.Fprintf(w, "Why: %s\n", why)
	}
	if len(decidedBy) > 0 {
		fmt.Fprintf(w, "Decided by: %s\n", strings.Join(decidedBy, ", "))
	}
}

// tellUser closes every refusal. Without it an agent treats a block as an
// obstacle and installs the nearest alternative instead.
func tellUser(w io.Writer) {
	fmt.Fprintln(w, "Do not install an alternative that does the same job. If you believe this is needed, stop and explain why to the user.")
}

func headline(e stack.Entry) string {
	var b strings.Builder
	b.WriteString(e.Name)
	if e.Version != "" {
		b.WriteString(" " + e.Version)
	}
	b.WriteString(" [" + e.Status + "]")
	if e.Package != "" {
		fmt.Fprintf(&b, " %s %s", e.Ecosystem, e.Package)
	}
	if len(e.Scope) > 0 {
		b.WriteString(" in " + scopeText(e))
	}
	return b.String()
}

func stopLine(e stack.Entry) string {
	var b strings.Builder
	switch e.Status {
	case stack.StatusHold:
		fmt.Fprintf(&b, "%s (hold): no new uses; existing ones may stay.", e.Name)
	case stack.StatusRetired:
		fmt.Fprintf(&b, "%s (retired): being removed.", e.Name)
	}
	if e.ReplacedBy != "" {
		fmt.Fprintf(&b, " Use %s instead.", e.ReplacedBy)
	}
	if e.Why != "" {
		fmt.Fprintf(&b, " %s", e.Why)
	}
	if len(e.DecidedBy) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(e.DecidedBy, ", "))
	}
	return b.String()
}

func avoidLine(a stack.Avoid) string {
	var b strings.Builder
	b.WriteString(a.Name + ":")
	if a.UseInstead != "" {
		fmt.Fprintf(&b, " use %s instead.", a.UseInstead)
	}
	if a.Why != "" {
		fmt.Fprintf(&b, " %s", a.Why)
	}
	if len(a.DecidedBy) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(a.DecidedBy, ", "))
	}
	return b.String()
}

func scopeText(e stack.Entry) string {
	if len(e.Scope) == 0 {
		return "the whole project"
	}
	return strings.Join(e.Scope, ", ")
}
