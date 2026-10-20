package migrations_test

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.yaml.in/yaml/v3"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
)

func TestTheAccessLogAcceptsExactlyTheEventsAndOutcomesOfTheContract(t *testing.T) {
	data, err := os.ReadFile("../contracts/openapi-bff.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Components struct {
			Schemas struct {
				AccessLogEntry struct {
					Properties map[string]struct {
						Enum []string `yaml:"enum"`
					} `yaml:"properties"`
				} `yaml:"AccessLogEntry"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	conn := connect(t, pgtest.ControlPlane(t).In("dashboard").As("migrator"))
	for _, column := range []string{"event", "outcome"} {
		want := slices.Sorted(slices.Values(contract.Components.Schemas.AccessLogEntry.Properties[column].Enum))
		rows, err := conn.Query(context.Background(), `
			SELECT DISTINCT m[1] FROM pg_constraint c, regexp_matches(pg_get_constraintdef(c.oid), '''([a-z_]+)''', 'g') m
			WHERE c.conrelid = 'dashboard.access_log'::regclass AND c.contype = 'c'
			  AND c.conkey = ARRAY[(SELECT attnum FROM pg_attribute
			    WHERE attrelid = 'dashboard.access_log'::regclass AND attname = $1)]
			ORDER BY 1`, column)
		if err != nil {
			t.Fatal(err)
		}
		got, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		if len(want) == 0 || !slices.Equal(got, want) {
			t.Errorf("%s: CHECK accepts %q, the contract lists %q", column, got, want)
		}
	}
}

func TestDashboardGrantsNothingToPublic(t *testing.T) {
	conn := connect(t, pgtest.ControlPlane(t).In("dashboard").As("migrator"))
	rows, err := conn.Query(context.Background(), `SELECT a.privilege_type FROM pg_database d,
		aclexplode(coalesce(d.datacl, acldefault('d', d.datdba))) a WHERE d.datname = 'dashboard' AND a.grantee = 0`)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := pgx.CollectRows(rows, pgx.RowTo[string]); err != nil || len(got) != 0 {
		t.Errorf("PUBLIC holds %q on dashboard (%v), want nothing", got, err)
	}
}
