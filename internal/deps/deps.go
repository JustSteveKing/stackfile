// Package deps reads what a project actually depends on, from the manifests
// and lockfiles its package managers write. Each reader returns nil when its
// ecosystem's manifest is absent: a project with no go.mod has no Go
// dependencies to check, which is not an error.
package deps

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Dependency is one installed package.
type Dependency struct {
	Name string `json:"name"`
	// Version is what the lockfile installed. Empty when there is no
	// lockfile, and then only presence can be checked.
	Version string `json:"version,omitempty"`
	// Direct is true when the project asks for the package itself, rather
	// than getting it through something else.
	Direct bool `json:"direct"`
}

// Inventory is one ecosystem's dependencies.
type Inventory struct {
	Ecosystem string `json:"ecosystem"`
	// Files are the manifest and lockfile read, relative to the project.
	Files []string `json:"files"`
	// Notes say what could not be read, such as a lockfile format not yet
	// supported. They are reported, never fatal.
	Notes []string `json:"notes,omitempty"`
	// Locked is true when versions came from a lockfile.
	Locked bool `json:"locked"`

	deps map[string]Dependency
	// fold matches names case-insensitively, which composer and npm do and
	// Go does not.
	fold bool
}

func newInventory(ecosystem string, fold bool) *Inventory {
	return &Inventory{Ecosystem: ecosystem, deps: map[string]Dependency{}, fold: fold}
}

func (inv *Inventory) key(name string) string {
	if inv.fold {
		return strings.ToLower(name)
	}
	return name
}

// add records a dependency. Directness is sticky: a package the project asks
// for stays direct even when it also appears deeper in the tree.
func (inv *Inventory) add(d Dependency) {
	k := inv.key(d.Name)
	if old, ok := inv.deps[k]; ok {
		d.Direct = d.Direct || old.Direct
		if d.Version == "" {
			d.Version = old.Version
		}
	}
	inv.deps[k] = d
}

// Find returns the dependency named name.
func (inv *Inventory) Find(name string) (Dependency, bool) {
	d, ok := inv.deps[inv.key(name)]
	return d, ok
}

// All returns every dependency, sorted by name.
func (inv *Inventory) All() []Dependency {
	out := make([]Dependency, 0, len(inv.deps))
	for _, d := range inv.deps {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Read returns an inventory for every ecosystem whose manifest is in dir.
func Read(dir string) ([]*Inventory, error) {
	var out []*Inventory
	for _, read := range []func(string) (*Inventory, error){readComposer, readNPM, readGo} {
		inv, err := read(dir)
		if err != nil {
			return nil, err
		}
		if inv != nil {
			out = append(out, inv)
		}
	}
	return out, nil
}

// readOptional returns a file's contents, or nil when it does not exist.
func readOptional(dir, name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}
