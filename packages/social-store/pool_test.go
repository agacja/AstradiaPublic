package socialstore

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The Store's sessions have its limits: a statement stops after 15 s and a
// transaction left idle ends after a minute. Checked on connections held at
// the same time, so each one is a different session.
func TestPoolSessionsHaveStatementAndIdleTransactionTimeouts(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	var conns []*pgxpool.Conn
	t.Cleanup(func() {
		for _, c := range conns {
			c.Release()
		}
	})
	for range 3 {
		c, err := s.pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	sessions := map[uint32]bool{}
	for _, c := range conns {
		var statement, idle string
		if err := c.QueryRow(ctx, `SELECT current_setting('statement_timeout'),
		                                  current_setting('idle_in_transaction_session_timeout')`).
			Scan(&statement, &idle); err != nil {
			t.Fatal(err)
		}
		if statement != "15s" || idle != "1min" {
			t.Fatalf("session timeouts = %q, %q; want 15s, 1min", statement, idle)
		}
		sessions[c.Conn().PgConn().PID()] = true
	}
	if len(sessions) != len(conns) {
		t.Fatalf("checked %d sessions, want %d", len(sessions), len(conns))
	}
}

// A connection is pinged on its way out of the pool only after half a minute
// unused, not after pgx's one second.
func TestPoolPingsOnlyConnectionsIdleForHalfAMinute(t *testing.T) {
	cfg, err := poolConfig("postgres://user@127.0.0.1:1/db")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for idle, want := range map[time.Duration]bool{
		0:                                false,
		2 * time.Second:                  false,
		pingAfterIdle:                    false,
		pingAfterIdle + time.Millisecond: true,
		time.Hour:                        true,
	} {
		if got := cfg.ShouldPing(ctx, pgxpool.ShouldPingParams{IdleDuration: idle}); got != want {
			t.Errorf("idle %v: ping = %v, want %v", idle, got, want)
		}
	}
}
