// Package schema carries the stackfile JSON Schema, which is the format's
// specification. Every other package validates against it rather than
// restating it, so a tool written in another language that uses the same
// file agrees with this one.
package schema

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// JSON is the schema document, exactly as published.
//
//go:embed stackfile.schema.json
var JSON []byte

// ID is the schema's $id, the URL editors resolve it from.
const ID = "https://raw.githubusercontent.com/JustSteveKing/stackfile/main/schema/stackfile.schema.json"

var (
	once     sync.Once
	compiled *jsonschema.Schema
	errLoad  error
)

// Compiled returns the schema ready to validate against. It is compiled once
// per process; the schema is embedded, so failing here is a bug in this
// repository and not in anyone's stackfile.
func Compiled() (*jsonschema.Schema, error) {
	once.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(JSON))
		if err != nil {
			errLoad = fmt.Errorf("embedded schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(ID, doc); err != nil {
			errLoad = fmt.Errorf("embedded schema: %w", err)
			return
		}
		compiled, errLoad = c.Compile(ID)
	})
	return compiled, errLoad
}
