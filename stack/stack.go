// Package stack loads a stackfile, validates it, and answers the questions an
// agent asks of it: what applies to this path, and may I add this package.
package stack

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// FileName is the one name a stackfile goes by. One canonical name is what
// lets an agent or a tool find it without being told where to look.
const FileName = "stackfile.yml"

// Statuses, in the order they are presented: what to use, what to try, what
// is undecided, what to stop using.
const (
	StatusAdopt    = "adopt"
	StatusTrial    = "trial"
	StatusProposed = "proposed"
	StatusHold     = "hold"
	StatusRetired  = "retired"
)

// Bases. All three are binding; the difference is how far the decision
// behind the entry has got. A basis covers the choice of technology, not its
// version, which has its own VersionDecidedBy.
const (
	// BasisDeclared: stated, with no decision record behind it yet.
	BasisDeclared = "declared"
	// BasisPending: chosen in a record that is not accepted yet.
	BasisPending = "pending"
	// BasisRatified: backed by accepted records.
	BasisRatified = "ratified"
)

// File is a parsed stackfile. Maps in the YAML are kept as ordered slices,
// because the order someone wrote their layers in is the order they think
// about them, and every output should follow it.
type File struct {
	Stackfile   int      `yaml:"stackfile" json:"stackfile,omitempty"`
	Project     string   `yaml:"project" json:"project,omitempty"`
	Description string   `yaml:"description" json:"description,omitempty"`
	Principles  []string `yaml:"principles" json:"principles,omitempty"`
	Layers      Layers   `yaml:"layers" json:"layers,omitempty"`
	Avoid       []Avoid  `yaml:"avoid" json:"avoid,omitempty"`
	Commands    Commands `yaml:"commands" json:"commands,omitempty"`

	// Path is where the file was read from, and Dir the directory scope
	// globs are relative to. Neither comes from the YAML.
	Path string `yaml:"-"`
	Dir  string `yaml:"-"`
}

// Layer is a named group of entries, such as backend or data.
type Layer struct {
	Name    string
	Entries []Entry
}

// Layers keeps the layers mapping in file order.
type Layers []Layer

// Entry is one technology the project uses, is trying, is deciding on, or is
// moving off.
type Entry struct {
	Name    string `yaml:"name" json:"name,omitempty"`
	Kind    string `yaml:"kind" json:"kind,omitempty"`
	Version string `yaml:"version" json:"version,omitempty"`
	// VersionDecidedBy names the records that set the version. Without it
	// the version is declared, whatever Basis says about the choice.
	VersionDecidedBy []string `yaml:"version_decided_by" json:"version_decided_by,omitempty"`
	Ecosystem        string   `yaml:"ecosystem" json:"ecosystem,omitempty"`
	Package          string   `yaml:"package" json:"package,omitempty"`
	Status           string   `yaml:"status" json:"status,omitempty"`
	Basis            string   `yaml:"basis" json:"basis,omitempty"`
	DecidedBy        []string `yaml:"decided_by" json:"decided_by,omitempty"`
	PendingOn        []string `yaml:"pending_on" json:"pending_on,omitempty"`
	Why              string   `yaml:"why" json:"why,omitempty"`
	Scope            []string `yaml:"scope" json:"scope,omitempty"`
	Rules            []string `yaml:"rules" json:"rules,omitempty"`
	Commands         Commands `yaml:"commands" json:"commands,omitempty"`
	Docs             string   `yaml:"docs" json:"docs,omitempty"`
	ReplacedBy       string   `yaml:"replaced_by" json:"replaced_by,omitempty"`
	Candidates       []string `yaml:"candidates" json:"candidates,omitempty"`
	Open             string   `yaml:"open" json:"open,omitempty"`

	// Layer is the name of the layer the entry sits in, and Line where it
	// starts in the file.
	Layer string `yaml:"-" json:"layer"`
	Line  int    `yaml:"-" json:"line"`
}

// Avoid is something not to introduce at all.
type Avoid struct {
	Name       string   `yaml:"name" json:"name,omitempty"`
	Ecosystem  string   `yaml:"ecosystem" json:"ecosystem,omitempty"`
	Package    string   `yaml:"package" json:"package,omitempty"`
	UseInstead string   `yaml:"use_instead" json:"use_instead,omitempty"`
	Why        string   `yaml:"why" json:"why,omitempty"`
	Basis      string   `yaml:"basis" json:"basis,omitempty"`
	DecidedBy  []string `yaml:"decided_by" json:"decided_by,omitempty"`
	PendingOn  []string `yaml:"pending_on" json:"pending_on,omitempty"`

	Line int `yaml:"-" json:"line"`
}

// Command is a named shell command.
type Command struct {
	Name string `json:"name"`
	Run  string `json:"run"`
}

// Commands keeps a commands mapping in file order.
type Commands []Command

// UnmarshalYAML reads the layers mapping without losing its order.
func (l *Layers) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: layers must be a mapping", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		name := n.Content[i].Value
		var entries []Entry
		if err := n.Content[i+1].Decode(&entries); err != nil {
			return err
		}
		for j := range entries {
			entries[j].Layer = name
			entries[j].Line = n.Content[i+1].Content[j].Line
			if entries[j].Basis == "" {
				entries[j].Basis = BasisDeclared
			}
		}
		*l = append(*l, Layer{Name: name, Entries: entries})
	}
	return nil
}

// UnmarshalYAML reads a commands mapping without losing its order.
func (c *Commands) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: commands must be a mapping", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		*c = append(*c, Command{Name: n.Content[i].Value, Run: n.Content[i+1].Value})
	}
	return nil
}

// Entries returns every entry in every layer, in file order.
func (f *File) Entries() []Entry {
	var all []Entry
	for _, l := range f.Layers {
		all = append(all, l.Entries...)
	}
	return all
}

// Entry finds an entry by name.
func (f *File) Entry(name string) (Entry, bool) {
	for _, e := range f.Entries() {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}
