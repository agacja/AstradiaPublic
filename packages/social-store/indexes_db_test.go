package socialstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// handleIndex reports whether the default-handle index exists and is valid.
func handleIndex(t *testing.T, fx *pgxpool.Pool) (exists, valid bool) {
	t.Helper()
	err := fx.QueryRow(context.Background(),
		`SELECT i.indisvalid FROM pg_index i WHERE i.indexrelid = to_regclass('idx_users_default_handle')`).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true, valid
}

// quickIndexLimits keep a test's attempt short.
var quickIndexLimits = IndexLimits{Lock: time.Second, Statement: 30 * time.Second}

// newIndexStore is newTestStore in a database of its own: an index built
// CONCURRENTLY waits for every older transaction in its database, and other
// tests, running at the same time, may hold some open.
func newIndexStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	dsn, fx := testDatabase(t)
	return openTestStore(t, dsn), fx
}

// The index is built, then found; the lookup uses it.
func TestTheDefaultHandleIndexIsBuiltOnce(t *testing.T) {
	s, fx := newIndexStore(t)
	ctx := context.Background()
	if outcome, err := s.BuildDefaultHandleIndex(ctx, quickIndexLimits); err != nil || outcome != IndexBuilt {
		t.Fatalf("first attempt: %q %v", outcome, err)
	}
	if exists, valid := handleIndex(t, fx); !exists || !valid {
		t.Fatalf("index exists %v valid %v", exists, valid)
	}
	if outcome, err := s.BuildDefaultHandleIndex(ctx, quickIndexLimits); err != nil || outcome != IndexPresent {
		t.Fatalf("second attempt: %q %v", outcome, err)
	}
	tx, err := fx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	rows, err := tx.Query(ctx, `EXPLAIN (COSTS OFF) `+defaultHandleSQL, "ab12")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), defaultHandleIndex) {
		t.Fatalf("the lookup does not use the index:\n%s", plan.String())
	}
}

// What a build stopped part-way leaves, an invalid index, is dropped and
// built again.
func TestAnInvalidDefaultHandleIndexIsBuiltAgain(t *testing.T) {
	s, fx := newIndexStore(t)
	ctx := context.Background()
	// A unique build over duplicate values fails exactly that way.
	addUsers(t, fx, "user_1_xxxxxxxxzz99", "user_2_yyyyyyyyzz99")
	if _, err := fx.Exec(ctx, `CREATE UNIQUE INDEX CONCURRENTLY idx_users_default_handle ON users (LOWER(RIGHT(id, 4))) WHERE nick IS NULL`); err == nil {
		t.Fatal("the fixture's build did not fail")
	}
	if exists, valid := handleIndex(t, fx); !exists || valid {
		t.Fatalf("fixture: exists %v valid %v", exists, valid)
	}
	if outcome, err := s.BuildDefaultHandleIndex(ctx, quickIndexLimits); err != nil || outcome != IndexRebuilt {
		t.Fatalf("%q %v", outcome, err)
	}
	if exists, valid := handleIndex(t, fx); !exists || !valid {
		t.Fatalf("exists %v valid %v", exists, valid)
	}
}

// One instance builds at a time: while another holds the build lock, an
// attempt does nothing.
func TestOnlyOneInstanceBuildsTheIndex(t *testing.T) {
	s, fx := newIndexStore(t)
	ctx := context.Background()
	held, err := fx.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if _, err := held.Exec(ctx, `SELECT pg_advisory_lock($1)`, indexBuildLockKey); err != nil {
		t.Fatal(err)
	}
	if outcome, err := s.BuildDefaultHandleIndex(ctx, quickIndexLimits); err != nil || outcome != IndexBusy {
		t.Fatalf("%q %v", outcome, err)
	}
	if exists, _ := handleIndex(t, fx); exists {
		t.Fatal("built while another instance held the lock")
	}
	if _, err := held.Exec(ctx, `SELECT pg_advisory_unlock($1)`, indexBuildLockKey); err != nil {
		t.Fatal(err)
	}
	if outcome, err := s.BuildDefaultHandleIndex(ctx, quickIndexLimits); err != nil || outcome != IndexBuilt {
		t.Fatalf("after the lock is free: %q %v", outcome, err)
	}
}

// A build held off by an open snapshot gives up at its lock limit, with a
// fixed code, instead of waiting for it.
func TestAnIndexBuildGivesUpBehindAnOpenSnapshot(t *testing.T) {
	s, fx := newIndexStore(t)
	ctx := context.Background()
	tx, err := fx.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT count(*) FROM users`); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = s.BuildDefaultHandleIndex(ctx, IndexLimits{Lock: 500 * time.Millisecond, Statement: 30 * time.Second})
	if code := IndexFailure(err); err == nil || (code != "sqlstate_55P03" && code != "sqlstate_57014") || time.Since(start) > 10*time.Second {
		t.Fatalf("behind an open snapshot: %v (%s) after %v", err, code, time.Since(start))
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if outcome, err := s.BuildDefaultHandleIndex(ctx, quickIndexLimits); err != nil || (outcome != IndexRebuilt && outcome != IndexBuilt) {
		t.Fatalf("once the snapshot is gone: %q %v", outcome, err)
	}
	if exists, valid := handleIndex(t, fx); !exists || !valid {
		t.Fatalf("exists %v valid %v", exists, valid)
	}
}

func TestIndexFailureIsAFixedCode(t *testing.T) {
	for err, want := range map[error]string{
		context.Canceled:         "canceled",
		context.DeadlineExceeded: "timeout",
		errIndexInvalid:          "still_invalid",
		errors.New("dial tcp: connection refused, secret detail"): "connection",
	} {
		if got := IndexFailure(err); got != want {
			t.Errorf("%v: %q, want %q", err, got, want)
		}
	}
}
