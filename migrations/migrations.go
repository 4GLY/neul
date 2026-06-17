package migrations

import "embed"

// FS contains SQLite migration files embedded into production binaries.
//
//go:embed *.sql
var FS embed.FS
