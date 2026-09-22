// Package migrations содержит SQL-миграции PostgreSQL, встраиваемые в бинарник.
package migrations

import "embed"

// FS - встроенная файловая система с файлами миграций.
//
//go:embed *.sql
var FS embed.FS
