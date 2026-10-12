package render

import (
	"errors"
	"fmt"
	"strings"

	"github.com/JustSteveKing/stackfile/stack"
)

// The markers bracket the generated block, so everything else in AGENTS.md
// stays the author's. The start marker says where the content comes from,
// because the first thing anyone does with a generated block is edit it.
const (
	StartMarker = "<!-- stackfile:start: generated from stackfile.yml by 'stackfile render'; edit that file, not this block -->"
	EndMarker   = "<!-- stackfile:end -->"
	startPrefix = "<!-- stackfile:start"
)

// Agents returns the block for AGENTS.md or CLAUDE.md, markers included.
//
// It is deliberately short. It is loaded into every session, so it carries
// what an agent must not get wrong (what to use, what not to add, what is
// still undecided) and points at 'stackfile explain' for the rules that
// apply where the agent is working.
func Agents(f *stack.File) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	line(StartMarker)
	line("## Stack")
	line("")
	line("`%s` is the source of truth for what this project builds with. If it and anything else disagree, it wins.", stack.FileName)
	line("")
	line("- Before working in a directory, run `stackfile explain <path>` for the rules that apply there.")
	line("- Before adding any dependency, run `stackfile can-i add <package>` and follow what it says.")

	if len(f.Principles) > 0 {
		line("")
		line("**Principles**, the tie-breakers for anything not covered:")
		line("")
		for _, p := range f.Principles {
			line("- %s", p)
		}
	}

	var use, trial, undecided, stop []stack.Entry
	for _, e := range f.Entries() {
		switch e.Status {
		case stack.StatusAdopt:
			use = append(use, e)
		case stack.StatusTrial:
			trial = append(trial, e)
		case stack.StatusProposed:
			undecided = append(undecided, e)
		default:
			stop = append(stop, e)
		}
	}

	if len(use) > 0 {
		line("")
		line("**Use**")
		line("")
		for _, e := range use {
			line("- %s", entryItem(e))
		}
	}
	if len(trial) > 0 {
		line("")
		line("**On trial**, only within the scope given:")
		line("")
		for _, e := range trial {
			line("- %s", entryItem(e))
		}
	}
	if len(undecided) > 0 {
		line("")
		line("**Undecided.** Do not choose; ask the user:")
		line("")
		for _, e := range undecided {
			item := fmt.Sprintf("%s: %s", e.Name, candidates(e.Candidates))
			if e.Open != "" {
				item += ". " + e.Open
			}
			line("- %s", item)
		}
	}
	if len(stop) > 0 || len(f.Avoid) > 0 {
		line("")
		line("**Do not add**")
		line("")
		for _, e := range stop {
			line("- %s", stopLine(e))
		}
		for _, a := range f.Avoid {
			line("- %s", avoidLine(a))
		}
	}
	if len(f.Commands) > 0 {
		line("")
		line("**Commands**")
		line("")
		for _, c := range f.Commands {
			line("- %s: `%s`", c.Name, c.Run)
		}
	}
	line(EndMarker)
	return b.String()
}

func entryItem(e stack.Entry) string {
	s := e.Name
	if e.Version != "" {
		s += " " + e.Version
	}
	s += " (" + e.Layer + ")"
	if len(e.Scope) > 0 {
		s += " in " + strings.Join(e.Scope, ", ")
	}
	if len(e.PendingOn) > 0 {
		s += ", pending " + strings.Join(e.PendingOn, ", ")
	}
	return s
}

// ErrUnclosed is returned by Splice when a start marker has no end marker.
// Guessing where the block ends would risk deleting the author's text.
var ErrUnclosed = errors.New("found the stackfile start marker but no end marker; fix the file by hand")

// Splice puts block into doc: in place of an existing block, or appended
// after a blank line when there is none.
func Splice(doc, block string) (string, error) {
	start := strings.Index(doc, startPrefix)
	if start < 0 {
		if doc == "" {
			return block, nil
		}
		return strings.TrimRight(doc, "\n") + "\n\n" + block, nil
	}
	rel := strings.Index(doc[start:], EndMarker)
	if rel < 0 {
		return "", ErrUnclosed
	}
	end := start + rel + len(EndMarker)
	if end < len(doc) && doc[end] == '\n' {
		end++
	}
	return doc[:start] + block + doc[end:], nil
}
