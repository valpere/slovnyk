// Package migrations embeds the append-only SQL migration inventory.
package migrations

import "embed"

// Files are checksummed and applied transactionally; never edit an applied SQL file.
//
//go:embed *.sql
var Files embed.FS
