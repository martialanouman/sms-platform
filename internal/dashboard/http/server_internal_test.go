package http

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	"github.com/martialanouman/sms-platform/internal/dashboard/postgres"
	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/migrations"
)

func TestTheAccessLogPageMapsEveryEntry(t *testing.T) {
	ctx := context.Background()
	pg := pgtest.ControlPlane(t).In("dashboard")
	migrator, err := pgx.Connect(ctx, pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close(ctx) })
	for _, s := range []string{string(migrations.Grants("dashboard")), `ALTER ROLE dashboard PASSWORD 'dashboard'`,
		`INSERT INTO dashboard.access_log (actor, event, target, outcome, at) SELECT NULL, 'login', NULL, 'failure', now() - interval '1 hour' FROM generate_series(1, 50)`,
		`INSERT INTO dashboard.access_log (actor, event, target, outcome) VALUES ('0190a000-0000-7000-8000-000000000001', 'access_link_requested', 'role-support', 'success')`,
	} {
		if _, err := migrator.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	pool, err := pgxpool.New(ctx, pg.As("dashboard"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	keys, err := dashboard.NewKeys(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	s := server{Config: Config{Service: dashboard.Service{Store: postgres.New(pool), Keys: keys}}}
	res, err := s.ListAccessLog(ctx, ListAccessLogRequestObject{})
	page, ok := res.(ListAccessLog200JSONResponse)
	if err != nil || !ok || len(page.Data) != 50 || page.NextCursor == nil {
		t.Fatalf("first page: %T %v", res, err)
	}
	first := page.Data[0]
	if first.Id == "" || first.At.IsZero() || first.Actor == nil || *first.Actor != "0190a000-0000-7000-8000-000000000001" ||
		first.Event != "access_link_requested" || first.Target == nil || *first.Target != "role-support" || first.Outcome != "success" {
		t.Errorf("newest entry %+v", first)
	}
	if last := page.Data[49]; last.Actor != nil || last.Target != nil || last.Outcome != "failure" || last.Event != "login" {
		t.Errorf("an anonymous failure %+v", last)
	}
	res, err = s.ListAccessLog(ctx, ListAccessLogRequestObject{Params: ListAccessLogParams{Cursor: page.NextCursor}})
	if next, ok := res.(ListAccessLog200JSONResponse); err != nil || !ok || len(next.Data) != 1 || next.NextCursor != nil {
		t.Fatalf("second page: %+v %v", res, err)
	}
	bad := "x"
	res, err = s.ListAccessLog(ctx, ListAccessLogRequestObject{Params: ListAccessLogParams{Cursor: &bad}})
	if refused, ok := res.(ListAccessLog422JSONResponse); err != nil || !ok || refused.Errors == nil || (*refused.Errors)[0].Field != "cursor" {
		t.Fatalf("bad cursor: %+v %v", res, err)
	}
	actor := openapi_types.UUID(uuid.MustParse("0190a000-0000-7000-8000-000000000001"))
	res, err = s.ListAccessLog(ctx, ListAccessLogRequestObject{Params: ListAccessLogParams{Actor: &actor}})
	if mine, ok := res.(ListAccessLog200JSONResponse); err != nil || !ok || len(mine.Data) == 0 ||
		slices.ContainsFunc(mine.Data, func(e AccessLogEntry) bool { return e.Actor == nil || *e.Actor != actor.String() }) {
		t.Fatalf("by actor: %+v %v", res, err)
	}
	later, earlier := time.Now().Add(time.Hour), time.Now().Add(-2*time.Hour)
	for name, params := range map[string]ListAccessLogParams{"from later": {FromDate: &later}, "to earlier": {ToDate: &earlier}} {
		res, err = s.ListAccessLog(ctx, ListAccessLogRequestObject{Params: params})
		if none, ok := res.(ListAccessLog200JSONResponse); err != nil || !ok || len(none.Data) != 0 {
			t.Errorf("%s: %+v %v", name, res, err)
		}
	}
}
