// Package postgres is the store of admin-api-svc on control_plane, through the queries sqlc generates.
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	"github.com/martialanouman/sms-platform/internal/adminapi/postgres/sqlc"
)

// Store runs the transactions of the control plane on a pool connected as admin_api_svc.
type Store struct {
	pool *pgxpool.Pool
}

// New is the store of pool.
func New(pool *pgxpool.Pool) Store {
	return Store{pool: pool}
}

var queries = sqlc.New()

// Tx runs fn in one transaction, its writes attributed to actor and requestID (I13).
func (s Store) Tx(ctx context.Context, actor, requestID string, fn func(adminapi.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(db pgx.Tx) error {
		if err := queries.Begin(ctx, db, sqlc.BeginParams{Actor: actor, RequestID: requestID}); err != nil {
			return err
		}
		return fn(tx{db: db})
	})
}

// Record writes e in a transaction of its own.
func (s Store) Record(ctx context.Context, e adminapi.Entry) error {
	return record(ctx, s.pool, e)
}

type tx struct {
	db pgx.Tx
}

func (t tx) Record(ctx context.Context, e adminapi.Entry) error {
	return record(ctx, t.db, e)
}

func record(ctx context.Context, db sqlc.DBTX, e adminapi.Entry) error {
	return queries.Record(ctx, db, sqlc.RecordParams{Operator: e.Operator, OperationID: e.OperationID, Target: e.Target,
		RequestID: e.RequestID, Status: int64(e.Status), Outcome: e.Outcome})
}

func (t tx) AuditLog(ctx context.Context, f adminapi.Filter, after *adminapi.Key, limit int) ([]adminapi.Entry, error) {
	p := sqlc.AuditLogParams{Operator: f.Operator, FromDate: f.From, ToDate: f.To, Lim: int64(limit)}
	if after != nil {
		p.AfterAt, p.AfterID = &after.At, &after.ID
	}
	rows, err := queries.AuditLog(ctx, t.db, p)
	if err != nil {
		return nil, err
	}
	page := make([]adminapi.Entry, len(rows))
	for i, r := range rows {
		page[i] = adminapi.Entry{ID: r.ID, Operator: r.Operator, OperationID: r.OperationID, Target: r.Target,
			RequestID: r.RequestID, Status: int(r.Status), Outcome: r.Outcome, At: r.At}
	}
	return page, nil
}

func (t tx) Changes(ctx context.Context, requestID string, after *adminapi.Key, limit int) ([]adminapi.Change, error) {
	p := sqlc.ChangesParams{RequestID: &requestID, Lim: int64(limit)}
	if after != nil {
		p.AfterAt, p.AfterID = &after.At, &after.ID
	}
	rows, err := queries.Changes(ctx, t.db, p)
	if err != nil {
		return nil, err
	}
	page := make([]adminapi.Change, len(rows))
	for i, r := range rows {
		page[i] = adminapi.Change{ID: r.ID, Actor: r.Actor, Table: r.TableName, PK: r.Pk, Op: r.Op, Before: r.Before,
			After: r.After, At: r.At}
	}
	return page, nil
}

func (t tx) Requested(ctx context.Context, requestID string) (bool, error) {
	return queries.Requested(ctx, t.db, &requestID)
}
