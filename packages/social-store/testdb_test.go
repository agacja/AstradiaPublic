package socialstore

// Database-backed tests run against a private, disposable copy of the schema.
//
// Each call creates a randomly named schema on the database named by
// PLATFORM_TEST_DATABASE_URL, builds a minimal `users` table holding only the
// columns this package reads (the calling service owns the real table) and
// applies the migrations in this package on top, exactly as they ship. The
// schema is dropped when the test ends. Nothing outside it is read or
// written, and the database must be a loopback one on a port other than 5432,
// so a developer's own PostgreSQL can never be the target by accident.

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.up.sql
var migrations embed.FS

// baseUsers is the part of a `users` table this package reads before its own
// migrations add their columns (dashboard preferences, avatar, nick, ...).
const baseUsers = `
CREATE TABLE users (
  id VARCHAR(64) PRIMARY KEY,
  email VARCHAR(255) UNIQUE NOT NULL,
  birth_date VARCHAR(10),
  birth_time VARCHAR(5),
  birth_latitude DECIMAL(10, 7),
  birth_longitude DECIMAL(10, 7),
  birth_timezone_offset INTEGER
);`

// testSchema returns a DSN whose connections see only a fresh schema, and a
// pool on that schema for arranging fixtures. The test is skipped when no
// disposable database is configured.
func testSchema(t *testing.T) (dsn string, pool *pgxpool.Pool) {
	t.Helper()
	raw := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires a disposable local PostgreSQL database (PLATFORM_TEST_DATABASE_URL)")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Port() == "5432" {
		t.Fatal("test database must use loopback and a nonstandard port")
	}

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	// Hex only, so the name is safe to place in DDL without quoting.
	schema := "socialtest_" + hex.EncodeToString(suffix)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop %s: %v", schema, err)
		}
		admin.Close()
	})

	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	dsn = u.String()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 4
	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Registered after the schema drop, so it runs first: no connection of
	// this pool is still using the schema when it is dropped.
	t.Cleanup(pool.Close)

	build(ctx, t, pool)
	return dsn, pool
}

// build creates the base users table and applies this package's migrations,
// in order.
func build(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, baseUsers); err != nil {
		t.Fatalf("base users: %v", err)
	}
	names, err := fs.Glob(migrations, "migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	for _, name := range names {
		sql, err := fs.ReadFile(migrations, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// testDatabase is testSchema in a database of its own, for tests that must
// not be affected by other tests running at the same time: an index built
// CONCURRENTLY waits for every older transaction in its database.
func testDatabase(t *testing.T) (dsn string, pool *pgxpool.Pool) {
	t.Helper()
	return database(t, "LOCALE 'C'")
}

// collatedSchema is testSchema in a database of its own whose default
// collation is the ICU locale given ("en-US"), the way a managed server is
// usually set up, rather than the byte order a test server initialised with C
// has. The test is skipped when the server cannot create one (no ICU, or no
// right to create databases).
func collatedSchema(t *testing.T, icuLocale string) (dsn string, pool *pgxpool.Pool) {
	t.Helper()
	if !localeRe.MatchString(icuLocale) {
		t.Fatalf("not a locale name: %q", icuLocale)
	}
	return database(t, "LOCALE_PROVIDER icu ICU_LOCALE '"+icuLocale+"' LOCALE 'C'")
}

var localeRe = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// database creates a randomly named database with the options given, builds
// the schema in it and drops it, connections and all, when the test ends.
func database(t *testing.T, options string) (dsn string, pool *pgxpool.Pool) {
	t.Helper()
	raw := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires a disposable local PostgreSQL database (PLATFORM_TEST_DATABASE_URL)")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Port() == "5432" {
		t.Fatal("test database must use loopback and a nonstandard port")
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	// Hex only, so the name is safe to place in DDL without quoting.
	name := "socialtest_" + hex.EncodeToString(suffix)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE template0 ENCODING 'UTF8' "+options); err != nil {
		admin.Close()
		t.Skipf("cannot create a database (%s): %v", options, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database %s: %v", name, err)
		}
		admin.Close()
	})

	u.Path = "/" + name
	dsn = u.String()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 4
	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	build(ctx, t, pool)
	return dsn, pool
}

// newTestStore opens the package's Store on a private schema, and returns a
// second pool on the same schema for arranging fixtures.
func newTestStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	dsn, fixtures := testSchema(t)
	return openTestStore(t, dsn), fixtures
}

// openTestStore opens the package's Store, with its own pool settings, on a
// test database.
func openTestStore(t *testing.T, dsn string) *Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// addUsers inserts bare accounts with the given ids.
func addUsers(t *testing.T, pool *pgxpool.Pool, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO users (id, email) VALUES ($1, $2)`, id, strings.ToLower(id)+"@example.test"); err != nil {
			t.Fatalf("add user %s: %v", id, err)
		}
	}
}

// befriend records a friendship edge from requester to addressee in the given
// status ("accepted", "pending" or "declined").
func befriend(t *testing.T, pool *pgxpool.Pool, requester, addressee, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO friendships (id, requester_id, addressee_id, status) VALUES ($1, $2, $3, $4)`,
		"fr_"+requester+"_"+addressee, requester, addressee, status); err != nil {
		t.Fatalf("befriend %s→%s: %v", requester, addressee, err)
	}
}
