// Package glob matches slash-separated paths against scope patterns. It is
// path.Match per segment plus "**" for any number of segments, which is all a
// scope needs, and it saves a dependency.
package glob

import (
	"errors"
	"path"
	"strings"
)

// Validate reports why a pattern cannot be used as a scope, or nil.
func Validate(pattern string) error {
	if strings.HasPrefix(pattern, "/") {
		return errors.New("scope globs are relative to the stackfile; drop the leading /")
	}
	for _, seg := range split(pattern) {
		if seg == ".." {
			return errors.New("scope globs cannot leave the stackfile's directory")
		}
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, ""); err != nil {
			return errors.New("malformed glob segment " + seg)
		}
	}
	return nil
}

// Match reports whether name, a slash-separated path relative to the
// stackfile, is matched by pattern.
func Match(pattern, name string) bool {
	return match(split(pattern), split(name))
}

// Below reports whether pattern could match something inside the directory
// dir. Asking what applies to app/ should turn up an entry scoped to
// app/Http/**, even though app itself is not matched.
func Below(pattern, dir string) bool {
	return below(split(pattern), split(dir))
}

func match(p, s []string) bool {
	if len(p) == 0 {
		return len(s) == 0
	}
	if p[0] == "**" {
		for i := 0; i <= len(s); i++ {
			if match(p[1:], s[i:]) {
				return true
			}
		}
		return false
	}
	if len(s) == 0 {
		return false
	}
	ok, _ := path.Match(p[0], s[0])
	return ok && match(p[1:], s[1:])
}

func below(p, s []string) bool {
	if len(s) == 0 {
		return len(p) > 0
	}
	if len(p) == 0 {
		return false
	}
	if p[0] == "**" {
		return true
	}
	ok, _ := path.Match(p[0], s[0])
	return ok && below(p[1:], s[1:])
}

// split turns a path into segments, ignoring ./ and empty segments, so that
// "./app//Http/" and "app/Http" are the same path. "." is the root.
func split(p string) []string {
	var out []string
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." {
			continue
		}
		out = append(out, seg)
	}
	return out
}
