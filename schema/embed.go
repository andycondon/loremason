// Package schema embeds the record schemas and the type registry.
package schema

import "embed"

// FS holds types.yaml, common.json and one JSON Schema per record type.
//
//go:embed types.yaml *.json
var FS embed.FS

// BaseURL is the prefix of every schema's $id.
const BaseURL = "https://github.com/andycondon/loremason/schema/"
