// Package postgres is the dashboard's store on its Postgres database, through the queries sqlc generates.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	"github.com/martialanouman/sms-platform/internal/dashboard/postgres/sqlc"
)

// Store runs the core's transactions on a pool connected as the dashboard role.
type Store struct {
	pool *pgxpool.Pool
}

// New is the store of pool.
func New(pool *pgxpool.Pool) Store {
	return Store{pool: pool}
}

// Tx runs fn in one transaction.
func (s Store) Tx(ctx context.Context, fn func(dashboard.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(db pgx.Tx) error {
		return fn(tx{db: db})
	})
}

var queries = sqlc.New()

type tx struct {
	db pgx.Tx
}

func found(err error) (bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (t tx) CreateOperator(ctx context.Context, email string) (string, bool, error) {
	id, err := queries.CreateOperator(ctx, t.db, email)
	created, err := found(err)
	return id, created, err
}

func (t tx) OperatorByEmail(ctx context.Context, email string) (dashboard.Operator, bool, error) {
	r, err := queries.OperatorByEmail(ctx, t.db, email)
	ok, err := found(err)
	return dashboard.Operator{ID: r.ID, PasswordHash: r.PasswordHash, Active: r.Active}, ok, err
}

func (t tx) SetPassword(ctx context.Context, operatorID, phc string) error {
	return queries.SetPassword(ctx, t.db, sqlc.SetPasswordParams{Phc: &phc, ID: operatorID})
}

func (t tx) Operators(ctx context.Context, id string, maxAttempts int) ([]dashboard.OperatorRecord, error) {
	rows, err := queries.Operators(ctx, t.db, sqlc.OperatorsParams{MaxAttempts: int64(maxAttempts), ID: optional(id)})
	out := make([]dashboard.OperatorRecord, len(rows))
	for i, r := range rows {
		out[i] = dashboard.OperatorRecord{ID: r.ID, Email: r.Email, DisplayName: r.DisplayName, Active: r.Active,
			Roles: make([]dashboard.RoleRef, len(r.RoleIds)), SecondFactor: r.SecondFactor}
		for j := range r.RoleIds {
			out[i].Roles[j] = dashboard.RoleRef{ID: r.RoleIds[j], Name: r.RoleNames[j]}
		}
		if r.LinkState != "" {
			out[i].Link = &dashboard.LinkState{Kind: dashboard.LinkKind(r.LinkKind), State: r.LinkState}
		}
	}
	return out, err
}

func (t tx) InsertOperator(ctx context.Context, email, displayName string) (string, bool, error) {
	id, err := queries.InsertOperator(ctx, t.db, sqlc.InsertOperatorParams{Email: email, DisplayName: displayName})
	created, err := found(err)
	return id, created, err
}

func (t tx) OperatorByID(ctx context.Context, id string) (dashboard.Operator, bool, error) {
	r, err := queries.OperatorByID(ctx, t.db, id)
	ok, err := found(err)
	return dashboard.Operator{ID: r.ID, PasswordHash: r.PasswordHash, Active: r.Active}, ok, err
}

func (t tx) SetOperatorStatus(ctx context.Context, id string, active bool) error {
	status := "disabled"
	if active {
		status = "active"
	}
	return queries.SetOperatorStatus(ctx, t.db, sqlc.SetOperatorStatusParams{Status: status, ID: id})
}

func (t tx) RolesOf(ctx context.Context, operatorID string) ([]string, error) {
	return queries.RolesOf(ctx, t.db, operatorID)
}

func (t tx) ExistingRoles(ctx context.Context, ids []string) ([]string, error) {
	return queries.ExistingRoles(ctx, t.db, ids)
}

func (t tx) SetRoles(ctx context.Context, operatorID string, roleIDs []string) error {
	if err := queries.ClearRoles(ctx, t.db, operatorID); err != nil {
		return err
	}
	return queries.AddRoles(ctx, t.db, sqlc.AddRolesParams{OperatorID: operatorID, RoleIds: roleIDs})
}

func (t tx) PermissionsOf(ctx context.Context, operatorID string) ([]string, error) {
	return queries.PermissionsOf(ctx, t.db, operatorID)
}

func (t tx) Roles(ctx context.Context, id string) ([]dashboard.RoleRecord, error) {
	rows, err := queries.Roles(ctx, t.db, optional(id))
	out := make([]dashboard.RoleRecord, len(rows))
	for i, r := range rows {
		out[i] = dashboard.RoleRecord{Role: dashboard.Role{Name: r.Name, Description: r.Description, Permissions: r.Permissions},
			ID: r.ID, Default: r.IsDefault, Holders: r.Holders}
	}
	return out, err
}

func (t tx) LockRole(ctx context.Context, id string) (bool, bool, error) {
	isDefault, err := queries.LockRole(ctx, t.db, id)
	ok, err := found(err)
	return isDefault, ok, err
}

func (t tx) InsertRole(ctx context.Context, r dashboard.Role) (string, bool, error) {
	id, err := queries.InsertRole(ctx, t.db, sqlc.InsertRoleParams{Name: r.Name, Description: r.Description, Permissions: r.Permissions})
	created, err := found(err)
	return id, created, err
}

func (t tx) UpdateRole(ctx context.Context, id, description string, permissions []string) error {
	return queries.UpdateRole(ctx, t.db, sqlc.UpdateRoleParams{Description: description, Permissions: permissions, ID: id})
}

func (t tx) DeleteRole(ctx context.Context, id string) error {
	return queries.DeleteRole(ctx, t.db, id)
}

func (t tx) SeedRole(ctx context.Context, r dashboard.Role) error {
	return queries.SeedRole(ctx, t.db, sqlc.SeedRoleParams{Name: r.Name, Description: r.Description, Permissions: r.Permissions})
}

func (t tx) EnsureOwner(ctx context.Context, email string) (string, bool, error) {
	id, err := queries.EnsureOwner(ctx, t.db, sqlc.EnsureOwnerParams{Email: email, Role: dashboard.Owner})
	granted, err := found(err)
	return id, granted, err
}

func (t tx) QueueLink(ctx context.Context, operatorID string, kind dashboard.LinkKind) error {
	return queries.QueueLink(ctx, t.db, sqlc.QueueLinkParams{OperatorID: operatorID, Kind: string(kind)})
}

func (t tx) ConsumeLink(ctx context.Context, digest []byte) (string, bool, error) {
	id, err := queries.ConsumeLink(ctx, t.db, digest)
	ok, err := found(err)
	return id, ok, err
}

func (t tx) NextDue(ctx context.Context, maxAttempts int) (dashboard.Due, bool, error) {
	r, err := queries.NextDue(ctx, t.db, int64(maxAttempts))
	ok, err := found(err)
	return dashboard.Due{OperatorID: r.OperatorID, Email: r.Email, Kind: dashboard.LinkKind(r.Kind)}, ok, err
}

func (t tx) MarkSent(ctx context.Context, operatorID string, digest []byte, ttl time.Duration) error {
	return queries.MarkSent(ctx, t.db, sqlc.MarkSentParams{Digest: digest, TtlSeconds: ttl.Seconds(), OperatorID: operatorID})
}

func (t tx) MarkFailed(ctx context.Context, operatorID string, base, ceiling time.Duration) error {
	return queries.MarkFailed(ctx, t.db, sqlc.MarkFailedParams{BaseSeconds: base.Seconds(), CeilingSeconds: ceiling.Seconds(), OperatorID: operatorID})
}

func (t tx) Undeliverable(ctx context.Context, maxAttempts int) (int, error) {
	n, err := queries.Undeliverable(ctx, t.db, int64(maxAttempts))
	return int(n), err
}

func (t tx) Reserve(ctx context.Context, subject []byte, window time.Duration, limit int) (time.Duration, error) {
	_, err := queries.Reserve(ctx, t.db, sqlc.ReserveParams{Subject: subject, WindowSeconds: window.Seconds(), MaxFailures: int64(limit)})
	if reserved, err := found(err); reserved || err != nil {
		return 0, err
	}
	left, err := queries.LockedFor(ctx, t.db, sqlc.LockedForParams{WindowSeconds: window.Seconds(), Subject: subject})
	return time.Duration(left * float64(time.Second)), err
}

func (t tx) Forget(ctx context.Context, subject []byte) error {
	return queries.Forget(ctx, t.db, subject)
}

func (t tx) OpenSession(ctx context.Context, operatorID string, digest, challenge []byte, lifetime, challengeTTL time.Duration) (time.Time, error) {
	return queries.OpenSession(ctx, t.db, sqlc.OpenSessionParams{OperatorID: operatorID, Digest: digest,
		LifetimeSeconds: lifetime.Seconds(), Challenge: challenge, ChallengeSeconds: challengeTTL.Seconds()})
}

func (t tx) SessionOf(ctx context.Context, digest []byte, idle, fresh time.Duration) (dashboard.SessionView, bool, error) {
	r, err := queries.SessionOf(ctx, t.db, sqlc.SessionOfParams{FreshSeconds: fresh.Seconds(), IdleSeconds: idle.Seconds(), Digest: digest})
	ok, err := found(err)
	return dashboard.SessionView{Current: dashboard.Current{SessionID: r.ID, OperatorID: r.OperatorID, Email: r.Email,
		DisplayName: r.DisplayName, ExpiresAt: r.ExpiresAt, Elevated: r.Elevated, Fresh: r.Fresh, Totp: r.Totp,
		RecoveryCodesRemaining: int(r.RecoveryCodesRemaining), Passkeys: int(r.Passkeys), Permissions: r.Permissions}, Live: r.Live}, ok, err
}

func (t tx) Touch(ctx context.Context, digest []byte, idle time.Duration) (bool, error) {
	n, err := queries.Touch(ctx, t.db, sqlc.TouchParams{Digest: digest, IdleSeconds: idle.Seconds()})
	return n == 1, err
}

func (t tx) CloseSession(ctx context.Context, digest []byte) (string, bool, error) {
	id, err := queries.CloseSession(ctx, t.db, digest)
	ok, err := found(err)
	return id, ok, err
}

func (t tx) CloseSessions(ctx context.Context, operatorID string) error {
	return queries.CloseSessions(ctx, t.db, operatorID)
}

func (t tx) CurrentStep(ctx context.Context) (int64, error) {
	return queries.CurrentStep(ctx, t.db)
}

func (t tx) Factors(ctx context.Context, operatorID string) ([]dashboard.TotpFactor, error) {
	rows, err := queries.Factors(ctx, t.db, operatorID)
	factors := make([]dashboard.TotpFactor, len(rows))
	for i, r := range rows {
		factors[i] = dashboard.TotpFactor{ID: r.ID, Sealed: r.Sealed, LastStep: r.LastStep, Confirmed: r.Confirmed}
	}
	return factors, err
}

func (t tx) AddPending(ctx context.Context, operatorID string, sealed []byte, codes [][]byte) error {
	id, err := queries.AddPending(ctx, t.db, sqlc.AddPendingParams{OperatorID: operatorID, Sealed: sealed})
	if err != nil {
		return err
	}
	if err := queries.DropRecoveryCodes(ctx, t.db, id); err != nil {
		return err
	}
	rows := make([]sqlc.AddRecoveryCodesParams, len(codes))
	for i, c := range codes {
		rows[i] = sqlc.AddRecoveryCodesParams{FactorID: id, Digest: c}
	}
	_, err = queries.AddRecoveryCodes(ctx, t.db, rows)
	return err
}

func (t tx) ConsumeStep(ctx context.Context, factorID string, step int64) error {
	return queries.ConsumeStep(ctx, t.db, sqlc.ConsumeStepParams{Step: step, ID: factorID})
}

func (t tx) Confirm(ctx context.Context, operatorID, factorID string) (bool, error) {
	n, err := queries.DropConfirmed(ctx, t.db, operatorID)
	if err != nil {
		return false, err
	}
	return n > 0, queries.Confirm(ctx, t.db, factorID)
}

func (t tx) UseRecoveryCode(ctx context.Context, factorID string, digest []byte) (bool, error) {
	n, err := queries.UseRecoveryCode(ctx, t.db, sqlc.UseRecoveryCodeParams{FactorID: factorID, Digest: digest})
	return n == 1, err
}

func (t tx) DropFactors(ctx context.Context, operatorID string) (bool, error) {
	apps, err := queries.DropFactors(ctx, t.db, operatorID)
	if err != nil {
		return false, err
	}
	keys, err := queries.DropPasskeys(ctx, t.db, operatorID)
	return apps+keys > 0, err
}

func (t tx) OpenCeremony(ctx context.Context, sessionID string, purpose dashboard.Purpose, ceremony []byte) error {
	return queries.OpenCeremony(ctx, t.db, sqlc.OpenCeremonyParams{SessionID: sessionID, Purpose: string(purpose), Ceremony: ceremony})
}

func (t tx) TakeCeremony(ctx context.Context, sessionID string, purpose dashboard.Purpose) ([]byte, bool, error) {
	ceremony, err := queries.TakeCeremony(ctx, t.db, sqlc.TakeCeremonyParams{SessionID: sessionID, Purpose: string(purpose)})
	ok, err := found(err)
	return ceremony, ok, err
}

func (t tx) Passkeys(ctx context.Context, operatorID string) ([]dashboard.StoredPasskey, error) {
	rows, err := queries.Passkeys(ctx, t.db, operatorID)
	keys := make([]dashboard.StoredPasskey, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, dashboard.StoredPasskey{Passkey: dashboard.Passkey{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt}, Credential: r.Credential})
	}
	return keys, err
}

func (t tx) AddPasskey(ctx context.Context, operatorID string, credentialID []byte, name string, signCount uint32, credential []byte) (string, bool, error) {
	id, err := queries.AddPasskey(ctx, t.db, sqlc.AddPasskeyParams{OperatorID: operatorID, CredentialID: credentialID, Name: name,
		SignCount: int64(signCount), Credential: credential})
	added, err := found(err)
	return id, added, err
}

func (t tx) AdvancePasskey(ctx context.Context, operatorID string, credentialID []byte, signCount uint32, credential []byte) (bool, error) {
	n, err := queries.AdvancePasskey(ctx, t.db, sqlc.AdvancePasskeyParams{OperatorID: operatorID, CredentialID: credentialID,
		SignCount: int64(signCount), Credential: credential})
	return n == 1, err
}

func (t tx) RemovePasskey(ctx context.Context, operatorID, id string) error {
	return queries.RemovePasskey(ctx, t.db, sqlc.RemovePasskeyParams{ID: id, OperatorID: operatorID})
}

func (t tx) Elevate(ctx context.Context, digest, challenge, newDigest []byte, idle time.Duration) (bool, error) {
	n, err := queries.Elevate(ctx, t.db, sqlc.ElevateParams{NewDigest: newDigest, Digest: digest, Challenge: challenge,
		IdleSeconds: idle.Seconds()})
	return n == 1, err
}

func (t tx) Purge(ctx context.Context, idle, window time.Duration) error {
	if err := queries.PurgeSessions(ctx, t.db, idle.Seconds()); err != nil {
		return err
	}
	return queries.PurgeCounters(ctx, t.db, window.Seconds())
}

func (t tx) Log(ctx context.Context, e dashboard.Entry) error {
	return queries.Log(ctx, t.db, sqlc.LogParams{Actor: optional(e.Actor), Event: string(e.Event), Target: optional(e.Target), Success: e.Success})
}

func (t tx) AccessLog(ctx context.Context, before *dashboard.Entry, limit int) ([]dashboard.Entry, error) {
	arg := sqlc.AccessLogParams{PageSize: int64(limit)}
	if before != nil {
		arg.BeforeAt, arg.BeforeID = &before.At, &before.ID
	}
	rows, err := queries.AccessLog(ctx, t.db, arg)
	entries := make([]dashboard.Entry, len(rows))
	for i, r := range rows {
		entries[i] = dashboard.Entry{ID: r.ID, At: r.At, Actor: r.Actor, Event: dashboard.Event(r.Event), Target: r.Target, Success: r.Success}
	}
	return entries, err
}
