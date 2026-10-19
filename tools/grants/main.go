// Command grants renders the Postgres roles and grants of the services from contracts/grants.
package main

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/martialanouman/sms-platform/internal/schema"
	"github.com/martialanouman/sms-platform/migrations"
)

const out = "migrations/grants/control_plane.sql"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	data, err := fs.ReadFile(migrations.ControlPlane(), "audit.yaml")
	if err != nil {
		return err
	}
	classification, err := schema.ParseClassification(data)
	if err != nil {
		return err
	}
	declared, err := schema.LoadGrants(os.DirFS("contracts/grants"), classification)
	if err != nil {
		return err
	}
	if err := os.MkdirAll("migrations/grants", 0o750); err != nil {
		return err
	}
	return os.WriteFile(out, schema.GrantsSQL(declared), 0o600)
}
