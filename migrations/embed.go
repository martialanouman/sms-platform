// Package migrations embeds the goose migrations of each database, one directory per database.
package migrations

import (
	"embed"
	"io/fs"
)

// Databases lists the Postgres databases of the platform, each a directory of migrations and a grants file.
var Databases = []string{"control_plane", "dashboard"}

//go:embed control_plane dashboard grants
var files embed.FS

// Grants is the generated grants file of database, applied by catalog-apply.
func Grants(database string) []byte {
	data, err := fs.ReadFile(files, "grants/"+database+".sql")
	if err != nil {
		panic(err)
	}
	return data
}

// Database is the directory of database: its migrations and its audit classification.
func Database(name string) fs.FS {
	sub, err := fs.Sub(files, name)
	if err != nil {
		panic(err)
	}
	return sub
}
