package stack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/JustSteveKing/stackfile/schema"
)

// ErrNotFound is returned by Find when no stackfile exists in the directory
// or any parent of it.
var ErrNotFound = errors.New("no " + FileName + " found here or in any parent directory")

// Find returns the path of the nearest stackfile, starting at dir and walking
// up. An agent can run stackfile from anywhere in a repository, the same way
// git finds its own directory.
func Find(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, FileName)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}

// Load reads and validates the stackfile at path. The file is returned only
// when it passed the schema; diagnostics are returned either way.
func Load(path string) (*File, Diagnostics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	f, ds := Parse(data)
	if f != nil {
		f.Path = abs
		f.Dir = filepath.Dir(abs)
	}
	return f, ds, nil
}

// Parse validates a stackfile in three passes: YAML that is safe to read, a
// document that matches the schema, and a stack that makes sense as a whole.
// The schema pass runs whenever the YAML parsed, so one run reports as much
// as it can; the semantic pass needs the schema to have passed, because it
// works on the decoded types.
//
// The file is returned when the first two passed. Semantic errors still come
// back with a file, so a caller can decide whether to act on it; the commands
// refuse to.
func Parse(data []byte) (*File, Diagnostics) {
	root, ds := parseYAML(data)
	if root == nil {
		return nil, ds
	}

	ds = append(ds, checkSchema(root, ds)...)
	if ds.HasErrors() {
		ds.sort()
		return nil, ds
	}

	var f File
	if err := root.Decode(&f); err != nil {
		// The schema has already accepted the document, so this is a gap
		// between the schema and the Go types, not the user's mistake.
		return nil, append(ds, Diagnostic{Severity: SeverityError, Line: 1, Column: 1, Message: "internal: " + err.Error()})
	}
	for i := range f.Avoid {
		f.Avoid[i].Line = nodeAt(root, []string{"avoid", strconv.Itoa(i)}).Line
		if f.Avoid[i].Basis == "" {
			f.Avoid[i].Basis = BasisDeclared
		}
	}

	ds = append(ds, checkSemantics(&f, root)...)
	ds.sort()
	return &f, ds
}

// parseYAML reads exactly one YAML document and rejects the features that
// make a file hard for an agent to edit safely: anchors, aliases and merge
// keys. It also catches versions written as numbers, before the schema
// reports them less helpfully.
func parseYAML(data []byte) (*yaml.Node, Diagnostics) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, Diagnostics{{Severity: SeverityError, Line: 1, Column: 1, Message: "the file is empty"}}
		}
		return nil, Diagnostics{yamlError(err)}
	}
	var next yaml.Node
	if err := dec.Decode(&next); err == nil {
		return nil, Diagnostics{{Severity: SeverityError, Line: next.Line, Column: next.Column,
			Message: "a stackfile is one YAML document; remove the second '---' and what follows it"}}
	}

	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}

	var ds Diagnostics
	walk(root, nil, func(n *yaml.Node, path []string) {
		switch {
		case n.Kind == yaml.AliasNode:
			ds = append(ds, Diagnostic{Severity: SeverityError, Line: n.Line, Column: n.Column, Path: display(path),
				Message: "aliases (*" + n.Value + ") are not allowed; write the value out so each entry reads on its own"})
		case n.Anchor != "":
			ds = append(ds, Diagnostic{Severity: SeverityError, Line: n.Line, Column: n.Column, Path: display(path),
				Message: "anchors (&" + n.Anchor + ") are not allowed; write the value out so each entry reads on its own"})
		case n.Tag == "!!merge":
			ds = append(ds, Diagnostic{Severity: SeverityError, Line: n.Line, Column: n.Column, Path: display(path),
				Message: "merge keys (<<) are not allowed; write the value out so each entry reads on its own"})
		}
		if len(path) > 0 && path[len(path)-1] == "version" && n.Kind == yaml.ScalarNode && (n.Tag == "!!int" || n.Tag == "!!float") {
			ds = append(ds, Diagnostic{Severity: SeverityError, Line: n.Line, Column: n.Column, Path: display(path),
				Message: fmt.Sprintf("version %s is read as a number%s; quote it: version: %q", n.Value, numberNote(n), n.Value)})
		}
	})
	return root, ds
}

// numberNote says what YAML turned a version into, when that differs from
// what was written. 1.10 becoming 1.1 is the mistake worth showing.
func numberNote(n *yaml.Node) string {
	if n.Tag != "!!float" {
		return ""
	}
	f, err := strconv.ParseFloat(n.Value, 64)
	if err != nil {
		return ""
	}
	if s := strconv.FormatFloat(f, 'f', -1, 64); s != n.Value {
		return " (" + s + ")"
	}
	return ""
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

func yamlError(err error) Diagnostic {
	d := Diagnostic{Severity: SeverityError, Line: 1, Column: 1, Message: strings.TrimPrefix(err.Error(), "yaml: ")}
	if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
		d.Line, _ = strconv.Atoi(m[1])
	}
	return d
}

// checkSchema validates the document against the embedded schema and places
// each failure on the line it is about. Failures at a path the YAML pass has
// already reported are dropped, so a quoted-version hint is not followed by
// "got number, want string" for the same value.
func checkSchema(root *yaml.Node, already Diagnostics) Diagnostics {
	sch, err := schema.Compiled()
	if err != nil {
		return Diagnostics{{Severity: SeverityError, Line: 1, Column: 1, Message: err.Error()}}
	}
	err = sch.Validate(toValue(root))
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return Diagnostics{{Severity: SeverityError, Line: 1, Column: 1, Message: err.Error()}}
	}

	reported := map[string]bool{}
	for _, d := range already {
		reported[d.Path] = true
	}
	p := message.NewPrinter(language.English)
	seen := map[string]bool{}
	var ds Diagnostics
	for _, leaf := range leaves(ve) {
		path := leaf.InstanceLocation
		n := nodeAt(root, path)
		// An unknown key is reported where the key is, not where the
		// object containing it starts.
		if ap, ok := leaf.ErrorKind.(*kind.AdditionalProperties); ok && len(ap.Properties) > 0 {
			if k := keyNode(n, ap.Properties[0]); k != nil {
				n = k
			}
		}
		d := Diagnostic{Severity: SeverityError, Line: n.Line, Column: n.Column, Path: display(path), Message: schemaMessage(leaf.ErrorKind, p)}
		if reported[d.Path] || seen[d.String()] {
			continue
		}
		seen[d.String()] = true
		ds = append(ds, d)
	}
	return ds
}

// requiredBecause explains the properties the schema only requires in
// combination with something else, which "missing property" leaves a reader
// to work out from the schema.
var requiredBecause = map[string]string{
	"candidates":  "a proposed entry needs candidates, an open question, or both",
	"open":        "a proposed entry needs candidates, an open question, or both",
	"replaced_by": "a retired entry needs replaced_by: the entry to use instead",
	"decided_by":  "basis: ratified needs decided_by: the accepted ADRs or RFCs behind it",
	"pending_on":  "basis: pending needs pending_on: the RFCs or ADRs, not yet accepted, that will decide it",
}

// schemaMessage rewords the failures people hit most into what to do about
// them, and leaves the rest as the validator says them.
func schemaMessage(k jsonschema.ErrorKind, p *message.Printer) string {
	switch k := k.(type) {
	case *kind.AdditionalProperties:
		return fmt.Sprintf("unknown field %s; check the spelling, or prefix a custom field with x-", quoteAll(k.Properties))
	case *kind.DependentRequired:
		if k.Prop == "ecosystem" || k.Prop == "package" {
			return fmt.Sprintf("%s needs %s too; ecosystem and package go together, so give both or neither", k.Prop, quoteAll(k.Missing))
		}
		return fmt.Sprintf("%s needs %s too", k.Prop, quoteAll(k.Missing))
	case *kind.MaxLength:
		return fmt.Sprintf("%d characters, and the limit is %d; keep it to one sentence and move the detail to a docs page", k.Got, k.Want)
	case *kind.Required:
		if len(k.Missing) == 1 {
			if why, ok := requiredBecause[k.Missing[0]]; ok {
				return why
			}
		}
	}
	return k.LocalizedString(p)
}

func quoteAll(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = "'" + s + "'"
	}
	return strings.Join(q, ", ")
}

// leaves returns the errors at the bottom of the tree. The ones above them
// say only that a subschema failed, which places nothing and fixes nothing.
func leaves(e *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(e.Causes) == 0 {
		return []*jsonschema.ValidationError{e}
	}
	var out []*jsonschema.ValidationError
	for _, c := range e.Causes {
		out = append(out, leaves(c)...)
	}
	return out
}

func keyNode(obj *yaml.Node, key string) *yaml.Node {
	if obj.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(obj.Content); i += 2 {
		if obj.Content[i].Value == key {
			return obj.Content[i]
		}
	}
	return nil
}

// toValue converts a node into the plain values the validator expects. It is
// written out rather than going through Decode so that the scalar types are
// exactly the ones the YAML pass saw.
func toValue(n *yaml.Node) any {
	switch n.Kind {
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			m[n.Content[i].Value] = toValue(n.Content[i+1])
		}
		return m
	case yaml.SequenceNode:
		s := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			s = append(s, toValue(c))
		}
		return s
	case yaml.AliasNode:
		return toValue(n.Alias)
	}
	switch n.Tag {
	case "!!null":
		return nil
	case "!!bool":
		b, _ := strconv.ParseBool(n.Value)
		return b
	case "!!int":
		if i, err := strconv.ParseInt(n.Value, 0, 64); err == nil {
			return i
		}
	case "!!float":
		if f, err := strconv.ParseFloat(n.Value, 64); err == nil {
			return f
		}
	}
	return n.Value
}

// walk visits every value node with the path to it.
func walk(n *yaml.Node, path []string, fn func(*yaml.Node, []string)) {
	fn(n, path)
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Tag == "!!merge" {
				fn(n.Content[i], path)
			}
			walk(n.Content[i+1], append(append([]string(nil), path...), n.Content[i].Value), fn)
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			walk(c, append(append([]string(nil), path...), strconv.Itoa(i)), fn)
		}
	}
}

// nodeAt follows a path into the document, stopping at the deepest node that
// exists. A missing property is reported on the object that lacks it.
func nodeAt(n *yaml.Node, path []string) *yaml.Node {
	for _, tok := range path {
		var next *yaml.Node
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == tok {
					next = n.Content[i+1]
					break
				}
			}
		case yaml.SequenceNode:
			if i, err := strconv.Atoi(tok); err == nil && i >= 0 && i < len(n.Content) {
				next = n.Content[i]
			}
		}
		if next == nil {
			return n
		}
		n = next
	}
	return n
}

// display renders a path the way someone would write it: layers.backend[0].
func display(path []string) string {
	var b strings.Builder
	for _, tok := range path {
		if _, err := strconv.Atoi(tok); err == nil {
			b.WriteString("[" + tok + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(tok)
	}
	return b.String()
}
