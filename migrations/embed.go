// Package migrations embeds the SQL schema migrations applied by the database package.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
