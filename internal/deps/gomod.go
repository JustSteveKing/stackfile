package deps

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// readGo reads go.mod. Since Go 1.17 it lists every module the build needs,
// with "// indirect" on those the project does not import itself, and the
// versions are the ones selected, so go.mod is manifest and lockfile at once.
func readGo(dir string) (*Inventory, error) {
	b, err := readOptional(dir, "go.mod")
	if err != nil || b == nil {
		return nil, err
	}
	inv := newInventory("go", false)
	inv.Files = []string{"go.mod"}
	inv.Locked = true

	inBlock := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		indirect := strings.Contains(line, "// indirect")
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		switch {
		case line == "":
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		case line == "require (":
			inBlock = true
			continue
		case strings.HasPrefix(line, "require "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "require "))
		case !inBlock:
			continue // module, go, toolchain, replace, exclude, retract, tool
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("go.mod:%d: cannot read requirement %q", n, line)
		}
		inv.add(Dependency{Name: strings.Trim(fields[0], `"`), Version: fields[1], Direct: !indirect})
	}
	return inv, sc.Err()
}

// Module finds the module that provides a Go package path: the dependency
// whose path is the package or a prefix of it, the longest if several are.
// An entry may name github.com/danielgtaylor/huma/v2/adapters/humachi and
// still be the huma module.
func (inv *Inventory) Module(pkg string) (Dependency, bool) {
	var best Dependency
	found := false
	for _, d := range inv.deps {
		if (pkg == d.Name || strings.HasPrefix(pkg, d.Name+"/")) && len(d.Name) > len(best.Name) {
			best, found = d, true
		}
	}
	return best, found
}
