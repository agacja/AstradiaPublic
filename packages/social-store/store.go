// Package socialstore contains Astradia's extracted social persistence layer.
// Authentication, friendship/consent authorization and HTTP validation belong
// to the calling service; this package is not a public HTTP server.
package socialstore

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	// dsn is the address the pool was opened with, for the one statement
	// that runs on a connection of its own (BuildDefaultHandleIndex).
	dsn string
}

// Open connects to a caller-supplied database. It never migrates or seeds it.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := poolConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool, dsn: databaseURL}, nil
}

// pingAfterIdle is how long a pooled connection may sit unused before it is
// pinged on its way out of the pool. pgx's own rule pings after one second,
// which costs most requests a round trip. A connection that dies within this
// window fails one query instead of being replaced unseen, and the pool then
// closes it.
const pingAfterIdle = 30 * time.Second

// poolConfig is the pool every Store uses.
func poolConfig(databaseURL string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 15
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	// A session left idle inside a transaction keeps its locks and its pool
	// connection; PostgreSQL ends it after a minute.
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "60000"
	cfg.ConnConfig.RuntimeParams["application_name"] = "astradia-public-social"
	cfg.ShouldPing = func(_ context.Context, p pgxpool.ShouldPingParams) bool {
		return p.IdleDuration > pingAfterIdle
	}
	return cfg, nil
}

func (s *Store) Close() { s.pool.Close() }
