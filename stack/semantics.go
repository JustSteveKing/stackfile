package stack

import (
	"fmt"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/JustSteveKing/stackfile/internal/glob"
)

// checkSemantics finds what the schema cannot see: problems that span
// entries, and fields that are valid on their own but contradict the rest of
// the entry.
func checkSemantics(f *File, root *yaml.Node) Diagnostics {
	var ds Diagnostics
	at := func(sev Severity, path []string, format string, args ...any) {
		n := nodeAt(root, path)
		ds = append(ds, Diagnostic{Severity: sev, Line: n.Line, Column: n.Column, Path: display(path), Message: fmt.Sprintf(format, args...)})
	}

	names := map[string]string{}    // entry name -> where it was first declared
	packages := map[string]string{} // ecosystem:package -> where

	for _, l := range f.Layers {
		for i, e := range l.Entries {
			p := []string{"layers", l.Name, strconv.Itoa(i)}
			here := display(p)

			if first, ok := names[e.Name]; ok {
				at(SeverityError, append(p, "name"), "%s is already declared at %s; names are unique across the file so replaced_by and avoid can refer to them", e.Name, first)
			} else {
				names[e.Name] = here
			}

			if e.Package != "" {
				key := e.Ecosystem + ":" + e.Package
				if first, ok := packages[key]; ok {
					at(SeverityError, append(p, "package"), "%s package %s is already declared at %s", e.Ecosystem, e.Package, first)
				} else {
					packages[key] = here
				}
			}

			for j, s := range e.Scope {
				if err := glob.Validate(s); err != nil {
					at(SeverityError, append(p, "scope", strconv.Itoa(j)), "%s", err)
				}
			}

			if len(e.DecidedBy) > 0 && e.Basis == BasisDeclared {
				at(SeverityWarning, append(p, "decided_by"), "has decided_by but basis is declared; set basis: ratified")
			}
			if e.Status != StatusProposed {
				if len(e.Candidates) > 0 {
					at(SeverityWarning, append(p, "candidates"), "candidates only apply while status is proposed; remove them now %s is %s", e.Name, e.Status)
				}
				if e.Open != "" {
					at(SeverityWarning, append(p, "open"), "open only applies while status is proposed; remove it now %s is %s", e.Name, e.Status)
				}
			}
			if e.ReplacedBy != "" && e.Status != StatusRetired && e.Status != StatusHold {
				at(SeverityWarning, append(p, "replaced_by"), "replaced_by only applies to hold or retired entries, and %s is %s", e.Name, e.Status)
			}
		}
	}

	// replaced_by is checked once every name is known, since the replacement
	// may be declared further down the file.
	for _, l := range f.Layers {
		for i, e := range l.Entries {
			if e.ReplacedBy == "" {
				continue
			}
			p := []string{"layers", l.Name, strconv.Itoa(i), "replaced_by"}
			switch r, ok := f.Entry(e.ReplacedBy); {
			case e.ReplacedBy == e.Name:
				at(SeverityError, p, "%s cannot replace itself", e.Name)
			case !ok:
				at(SeverityWarning, p, "%s is not an entry in this file, so an agent told to use it instead has no rules for it; declare it", e.ReplacedBy)
			case r.Status == StatusHold || r.Status == StatusRetired:
				at(SeverityWarning, p, "%s is replaced by %s, which is itself %s", e.Name, r.Name, r.Status)
			}
		}
	}

	for i, a := range f.Avoid {
		p := []string{"avoid", strconv.Itoa(i)}
		if where, ok := names[a.Name]; ok {
			at(SeverityError, append(p, "name"), "%s is both an entry (%s) and on the avoid list; to phase it out, set its status to hold or retired instead", a.Name, where)
		}
		if a.Package != "" {
			if where, ok := packages[a.Ecosystem+":"+a.Package]; ok {
				at(SeverityError, append(p, "package"), "%s package %s is both declared at %s and on the avoid list", a.Ecosystem, a.Package, where)
			}
		}
		if len(a.DecidedBy) > 0 && a.Basis == BasisDeclared {
			at(SeverityWarning, append(p, "decided_by"), "has decided_by but basis is declared; set basis: ratified")
		}
	}

	return ds
}
