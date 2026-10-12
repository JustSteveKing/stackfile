package deps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// readNPM reads package.json for what the project asks for, then the first
// lockfile it understands: package-lock.json, then bun.lock.
func readNPM(dir string) (*Inventory, error) {
	manifest, err := readOptional(dir, "package.json")
	if err != nil || manifest == nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(manifest, &m); err != nil {
		return nil, fmt.Errorf("package.json: %w", err)
	}

	inv := newInventory("npm", true)
	inv.Files = []string{"package.json"}
	for _, field := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
		var deps map[string]string
		if raw, ok := m[field]; ok {
			if err := json.Unmarshal(raw, &deps); err != nil {
				return nil, fmt.Errorf("package.json %s: %w", field, err)
			}
		}
		for name := range deps {
			inv.add(Dependency{Name: name, Direct: true})
		}
	}

	switch {
	case exists(dir, "package-lock.json"):
		return inv, readPackageLock(dir, inv)
	case exists(dir, "bun.lock"):
		return inv, readBunLock(dir, inv)
	case exists(dir, "bun.lockb"):
		inv.Notes = append(inv.Notes, "bun.lockb is binary; run 'bun install --save-text-lockfile' for a bun.lock that can be checked")
	case exists(dir, "pnpm-lock.yaml"):
		inv.Notes = append(inv.Notes, "pnpm-lock.yaml is not read yet, so versions and indirect dependencies are not checked")
	case exists(dir, "yarn.lock"):
		inv.Notes = append(inv.Notes, "yarn.lock is not read yet, so versions and indirect dependencies are not checked")
	default:
		inv.Notes = append(inv.Notes, "no lockfile, so versions and indirect dependencies are not checked")
	}
	return inv, nil
}

// readPackageLock reads lockfileVersion 2 and 3, whose "packages" map is
// keyed by install path: node_modules/a, node_modules/a/node_modules/b.
func readPackageLock(dir string, inv *Inventory) error {
	b, err := readOptional(dir, "package-lock.json")
	if err != nil {
		return err
	}
	var l struct {
		LockfileVersion int `json:"lockfileVersion"`
		Packages        map[string]struct {
			Version string `json:"version"`
			Name    string `json:"name"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(b, &l); err != nil {
		return fmt.Errorf("package-lock.json: %w", err)
	}
	if l.LockfileVersion < 2 {
		inv.Notes = append(inv.Notes, "package-lock.json is lockfileVersion 1; regenerate it with npm 7 or later to check versions")
		return nil
	}
	inv.Files = append(inv.Files, "package-lock.json")
	inv.Locked = true
	// A top-level install (node_modules/a) is the version the project
	// resolves; nested copies only fill in packages not seen at the top.
	topLevel := map[string]bool{}
	for _, path := range sortedKeys(l.Packages) {
		p := l.Packages[path]
		i := strings.LastIndex(path, "node_modules/")
		if i < 0 {
			continue // "" is the project itself
		}
		name := path[i+len("node_modules/"):]
		if p.Name != "" {
			name = p.Name // an alias: "node_modules/x": {"name": "real-package"}
		}
		nested := strings.Count(path, "node_modules/") > 1
		if nested && topLevel[inv.key(name)] {
			continue
		}
		if !nested {
			topLevel[inv.key(name)] = true
		}
		inv.add(Dependency{Name: name, Version: p.Version})
	}
	return nil
}

// readBunLock reads bun's text lockfile. It is JSON with trailing commas,
// and each package is ["name@version", ...] under a key that is the name,
// or parent/name for a nested copy.
func readBunLock(dir string, inv *Inventory) error {
	b, err := readOptional(dir, "bun.lock")
	if err != nil {
		return err
	}
	var l struct {
		Packages map[string][]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(stripTrailingCommas(b), &l); err != nil {
		return fmt.Errorf("bun.lock: %w", err)
	}
	inv.Files = append(inv.Files, "bun.lock")
	inv.Locked = true
	topLevel := map[string]bool{}
	for _, key := range sortedKeys(l.Packages) {
		entry := l.Packages[key]
		if len(entry) == 0 {
			continue
		}
		var spec string
		if err := json.Unmarshal(entry[0], &spec); err != nil {
			continue
		}
		at := strings.LastIndex(spec, "@")
		if at <= 0 {
			continue
		}
		name, ver := spec[:at], spec[at+1:]
		if strings.HasPrefix(ver, "npm:") {
			ver = ver[len("npm:"):]
		}
		nested := key != name
		if nested && topLevel[inv.key(name)] {
			continue
		}
		if !nested {
			topLevel[inv.key(name)] = true
		}
		inv.add(Dependency{Name: name, Version: ver})
	}
	return nil
}

// sortedKeys makes the walk deterministic: with two nested copies and no
// top-level one, the same copy wins on every run.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stripTrailingCommas removes commas before } or ], outside strings, which
// is all that separates bun.lock from JSON.
func stripTrailingCommas(b []byte) []byte {
	var out bytes.Buffer
	inString, escaped := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inString {
			out.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
		}
		if c == ',' {
			j := i + 1
			for j < len(b) && (b[j] == ' ' || b[j] == '\n' || b[j] == '\r' || b[j] == '\t') {
				j++
			}
			if j < len(b) && (b[j] == '}' || b[j] == ']') {
				continue
			}
		}
		out.WriteByte(c)
	}
	return out.Bytes()
}
