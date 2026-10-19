// Package migrations embeds identity schema changes for the migration command.
package migrations

import "embed"

// Files contains the identity SQL migration files.
//
//go:embed *.sql
var Files embed.FS
