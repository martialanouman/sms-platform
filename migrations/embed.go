// Package migrations embeds the goose migrations of each database, one directory per database.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed control_plane
var files embed.FS

//go:embed grants/control_plane.sql
var grants []byte

// Grants is the generated grants file of control_plane, applied by catalog-apply.
func Grants() []byte {
	return grants
}

// ControlPlane is the control_plane directory: its migrations and its audit classification.
func ControlPlane() fs.FS {
	sub, err := fs.Sub(files, "control_plane")
	if err != nil {
		panic(err)
	}
	return sub
}
