// Command grants renders, database by database, the Postgres roles and grants of the services from contracts/grants.
package main

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/martialanouman/sms-platform/internal/schema"
	"github.com/martialanouman/sms-platform/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	classifications := map[string]schema.Classification{}
	for _, database := range migrations.Databases {
		data, err := fs.ReadFile(migrations.Database(database), "audit.yaml")
		if err != nil {
			return err
		}
		if classifications[database], err = schema.ParseClassification(data); err != nil {
			return fmt.Errorf("%s: %w", database, err)
		}
	}
	declared, err := schema.LoadGrants(os.DirFS("contracts/grants"), classifications)
	if err != nil {
		return err
	}
	for _, database := range migrations.Databases {
		if err := os.WriteFile("migrations/grants/"+database+".sql", schema.GrantsSQL(database, declared), 0o600); err != nil {
			return err
		}
	}
	return nil
}
