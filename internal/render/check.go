package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/JustSteveKing/stackfile/internal/check"
)

// Check writes check's result: what was read, what was skipped and why, then
// each finding located at the stackfile line it concerns, so an editor or
// terminal can jump to it.
func Check(w io.Writer, stackfile string, res check.Result) {
	if len(res.Checked) == 0 {
		fmt.Fprintln(w, "No composer.json, package.json or go.mod found next to the stackfile; nothing to check.")
	} else {
		var parts []string
		for _, inv := range res.Checked {
			parts = append(parts, fmt.Sprintf("%s (%s)", inv.Ecosystem, strings.Join(inv.Files, ", ")))
		}
		fmt.Fprintf(w, "Checked %s.\n", strings.Join(parts, ", "))
	}
	for _, s := range res.Skipped {
		fmt.Fprintf(w, "Skipped %s: %s.\n", s.Ecosystem, s.Reason)
	}
	for _, inv := range res.Checked {
		for _, n := range inv.Notes {
			fmt.Fprintf(w, "Note %s: %s.\n", inv.Ecosystem, n)
		}
	}

	if len(res.Findings) == 0 {
		fmt.Fprintln(w, "\nNo problems found.")
		return
	}
	fmt.Fprintln(w)
	for _, f := range res.Findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", stackfile, f.Line)
		}
		fmt.Fprintf(w, "%s: %s: %s\n", loc, f.Severity, f.Message)
	}
	fmt.Fprintf(w, "\n%d error(s), %d warning(s), %d note(s)\n",
		res.Count(check.SeverityError), res.Count(check.SeverityWarning), res.Count(check.SeverityNote))
}
