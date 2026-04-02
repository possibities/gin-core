package migrations

import "embed"

// Files contains the SQL migration files embedded into the binary.
//
//go:embed *.sql
var Files embed.FS
