package postgres

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/martialanouman/sms-platform/internal/adminapi"
)

func DB(t adminapi.Tx) pgx.Tx {
	return t.(tx).db
}

func Pool(s Store) *pgxpool.Pool {
	return s.pool
}
