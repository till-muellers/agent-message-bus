// Package migrations embeds the numbered SQL schema files.
package migrations

import "embed"

// FS holds every *.sql migration file.
//
//go:embed *.sql
var FS embed.FS
