package schema

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Privilege is a table privilege a service may declare.
type Privilege string

// The privileges a service may declare; TRUNCATE would bypass audit_capture.
const (
	Select Privilege = "select"
	Insert Privilege = "insert"
	Update Privilege = "update"
	Delete Privilege = "delete"
)

const database = "control_plane"

var serviceName = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// Grants is what one service declares it reads or writes in control_plane, table by table.
type Grants struct {
	Service string
	Tables  map[string][]Privilege
}

// ParseGrants decodes the declaration of service and checks it against the audit classification.
func ParseGrants(service string, data []byte, classification Classification) (Grants, error) {
	if !serviceName.MatchString(service) {
		return Grants{}, fmt.Errorf("grants: service %q is not a service name", service)
	}
	var declared map[string]map[string][]Privilege
	if err := yaml.Unmarshal(data, &declared); err != nil {
		return Grants{}, fmt.Errorf("grants of %s: %w", service, err)
	}
	g := Grants{Service: service, Tables: map[string][]Privilege{}}
	for db, tables := range declared {
		if db != database {
			return Grants{}, fmt.Errorf("grants of %s: database %q is not %s", service, db, database)
		}
		for table, privileges := range tables {
			if _, ok := classification[table]; !ok {
				return Grants{}, fmt.Errorf("grants of %s: table %q is not classified", service, table)
			}
			if len(privileges) == 0 {
				return Grants{}, fmt.Errorf("grants of %s: table %q grants nothing", service, table)
			}
			for _, p := range privileges {
				if !slices.Contains([]Privilege{Select, Insert, Update, Delete}, p) {
					return Grants{}, fmt.Errorf("grants of %s: privilege %q on %q is not declarable", service, p, table)
				}
			}
			g.Tables[table] = privileges
		}
	}
	return g, nil
}

// Role is the Postgres role of a service.
func Role(service string) string {
	return strings.ReplaceAll(service, "-", "_")
}
