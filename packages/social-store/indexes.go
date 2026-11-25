package socialstore

import (
	"context"
	"errors"
	"hash/fnv"
	"net"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The default-handle lookup (defaultHandleSQL) is served by an index of its
// own, which the calling service builds with BuildDefaultHandleIndex after it
// has started.
//
// It is not a migration. An index built CONCURRENTLY waits for every older
// transaction in the database to end, so its build can take as long as the
// longest of them; inside golang-migrate a build stopped part-way (a health
// check that gave up, a cancelled deploy) left the schema version dirty, and
// a release that knew nothing of that version refused to start. Here the
// build runs in the background on a connection of its own, one instance at a
// time, gives up when it has waited too long, and is only ever an
// optimisation: until the index exists the lookup reads the whole users
// table, as it always has.
const (
	defaultHandleIndex    = "idx_users_default_handle"
	defaultHandleIndexSQL = `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_default_handle ON users (LOWER(RIGHT(id, 4))) WHERE nick IS NULL`
)

// IndexLimits bound one attempt at building the index. Lock is the longest
// it waits for any one lock, other transactions ending included; Statement
// is the longest any one statement runs. An attempt that hits either leaves
// at most an invalid index behind, which the next attempt drops and builds
// again.
type IndexLimits struct {
	Lock, Statement time.Duration
}

// DefaultIndexLimits bound one attempt for a caller with no reason to choose
// otherwise. Waiting at most 10 s at a time also bounds how long the build can
// hold off other changes to the table's definition.
var DefaultIndexLimits = IndexLimits{Lock: 10 * time.Second, Statement: 5 * time.Minute}

// What an attempt did, as fixed words for the log.
const (
	IndexPresent = "present" // already there and valid
	IndexBuilt   = "built"
	IndexRebuilt = "rebuilt" // an invalid one, left by a build stopped part-way, was dropped and built again
	IndexBusy    = "busy"    // another instance is building it
)

// indexBuildLockKey is the session advisory lock only the instance building
// the index holds, in a namespace of its own like the friendship keys.
var indexBuildLockKey = func() int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("index-build\x00" + defaultHandleIndex))
	return int64(h.Sum64())
}()

// BuildDefaultHandleIndex makes one attempt at the default-handle index and
// says what it did. On failure the error's IndexFailure is a fixed code fit
// for a log line.
func (s *Store) BuildDefaultHandleIndex(ctx context.Context, limits IndexLimits) (string, error) {
	cfg, err := pgx.ParseConfig(s.dsn)
	if err != nil {
		return "", err
	}
	cfg.RuntimeParams["application_name"] = "astradia-public-social-index"
	cfg.RuntimeParams["lock_timeout"] = strconv.FormatInt(limits.Lock.Milliseconds(), 10)
	cfg.RuntimeParams["statement_timeout"] = strconv.FormatInt(limits.Statement.Milliseconds(), 10)
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return "", err
	}
	// Closing the connection also lets go of the advisory lock.
	defer conn.Close(context.Background())

	var mine bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, indexBuildLockKey).Scan(&mine); err != nil {
		return "", err
	}
	if !mine {
		return IndexBusy, nil
	}
	exists, valid, err := indexState(ctx, conn, defaultHandleIndex)
	if err != nil {
		return "", err
	}
	if exists && valid {
		return IndexPresent, nil
	}
	outcome := IndexBuilt
	if exists {
		outcome = IndexRebuilt
		if _, err := conn.Exec(ctx, `DROP INDEX CONCURRENTLY IF EXISTS `+defaultHandleIndex); err != nil {
			return "", err
		}
	}
	if _, err := conn.Exec(ctx, defaultHandleIndexSQL); err != nil {
		return "", err
	}
	if _, valid, err := indexState(ctx, conn, defaultHandleIndex); err != nil || !valid {
		if err == nil {
			err = errIndexInvalid
		}
		return "", err
	}
	return outcome, nil
}

var errIndexInvalid = errors.New("store: the index is still invalid after its build")

func indexState(ctx context.Context, conn *pgx.Conn, name string) (exists, valid bool, err error) {
	err = conn.QueryRow(ctx,
		`SELECT i.indisvalid FROM pg_index i WHERE i.indexrelid = to_regclass($1)`, name).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	return err == nil, valid, err
}

// IndexFailure is a failed attempt as a fixed code for the log: the
// database's SQLSTATE (55P03 a lock it waited too long for, 57014 a statement
// that ran too long), or one word. Never the error's own text.
func IndexFailure(err error) string {
	var pgErr *pgconn.PgError
	var netErr net.Error
	switch {
	case errors.As(err, &pgErr):
		return "sqlstate_" + pgErr.Code
	case errors.Is(err, errIndexInvalid):
		return "still_invalid"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	}
	return "connection"
}
