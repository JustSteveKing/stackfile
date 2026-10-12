package stack

import (
	"fmt"
	"sort"
)

// Severity says whether a diagnostic stops the file being used.
type Severity string

const (
	// SeverityError means the file is wrong: an agent reading it would be
	// misled, so nothing should act on it until it is fixed.
	SeverityError Severity = "error"
	// SeverityWarning means the file works but is drifting: a stale field,
	// a reference to nothing, a decision recorded in the wrong place.
	SeverityWarning Severity = "warning"
)

// Diagnostic is one problem, located in the file.
type Diagnostic struct {
	Severity Severity `json:"severity"`
	Line     int      `json:"line"`
	Column   int      `json:"column"`
	// Path is where in the document the problem is, such as
	// layers.backend[0].version. Empty for the whole file.
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func (d Diagnostic) String() string {
	loc := ""
	if d.Path != "" {
		loc = d.Path + ": "
	}
	return fmt.Sprintf("%d:%d: %s: %s%s", d.Line, d.Column, d.Severity, loc, d.Message)
}

// Diagnostics is a set of problems, kept in file order.
type Diagnostics []Diagnostic

// HasErrors reports whether any diagnostic is an error.
func (ds Diagnostics) HasErrors() bool {
	for _, d := range ds {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Count returns the number of diagnostics at a severity.
func (ds Diagnostics) Count(s Severity) int {
	n := 0
	for _, d := range ds {
		if d.Severity == s {
			n++
		}
	}
	return n
}

func (ds Diagnostics) sort() {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].Line != ds[j].Line {
			return ds[i].Line < ds[j].Line
		}
		return ds[i].Column < ds[j].Column
	})
}
