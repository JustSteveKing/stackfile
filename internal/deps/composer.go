package deps

import (
	"encoding/json"
	"fmt"
	"strings"
)

// readComposer reads composer.json for what the project asks for, and
// composer.lock, when present, for what was installed.
func readComposer(dir string) (*Inventory, error) {
	manifest, err := readOptional(dir, "composer.json")
	if err != nil || manifest == nil {
		return nil, err
	}
	var m struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		return nil, fmt.Errorf("composer.json: %w", err)
	}

	inv := newInventory("composer", true)
	inv.Files = []string{"composer.json"}
	for _, req := range []map[string]string{m.Require, m.RequireDev} {
		for name := range req {
			if platform(name) {
				continue
			}
			inv.add(Dependency{Name: name, Direct: true})
		}
	}

	lock, err := readOptional(dir, "composer.lock")
	if err != nil {
		return nil, err
	}
	if lock == nil {
		inv.Notes = append(inv.Notes, "no composer.lock, so versions and indirect dependencies are not checked")
		return inv, nil
	}
	var l struct {
		Packages    []composerPackage `json:"packages"`
		PackagesDev []composerPackage `json:"packages-dev"`
	}
	if err := json.Unmarshal(lock, &l); err != nil {
		return nil, fmt.Errorf("composer.lock: %w", err)
	}
	inv.Files = append(inv.Files, "composer.lock")
	inv.Locked = true
	for _, p := range append(l.Packages, l.PackagesDev...) {
		inv.add(Dependency{Name: p.Name, Version: p.Version})
	}
	return inv, nil
}

type composerPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// platform reports whether a requirement is the PHP runtime or an extension,
// which composer checks but never installs.
func platform(name string) bool {
	switch name {
	case "php", "php-64bit", "php-ipv6", "php-zts", "php-debug", "hhvm", "composer", "composer-plugin-api", "composer-runtime-api":
		return true
	}
	return strings.HasPrefix(name, "ext-") || strings.HasPrefix(name, "lib-")
}
