package migrations

import "embed"

// Files contains notification migration scripts.
//
//go:embed *.sql
var Files embed.FS
