# Social persistence package

This is an extraction of Astradia's Go social data-access layer. It includes
profiles and preferences, friends and friend requests, synastry consent state,
avatars, wall comments, and unread markers. It contains no comparison/scoring
algorithm, bot execution, encrypted trading keys, or customer data.

The calling service must authenticate users, validate requests, and enforce
friendship and mutual consent before reading or writing protected data. Store
functions are not a complete authorization boundary. This package intentionally
does not start an HTTP server, provision accounts, or run migrations.

## Tests

Use Go 1.25+ and PostgreSQL 16. The database tests use synthetic identifiers
and refuse a non-loopback database or the usual production port. One creates
session-local temporary tables; the others (`social_db_test.go`) each get a
freshly named schema with a minimal `users` table and this package's
migrations applied on top (`testdb_test.go`), dropped when the test ends. They
include requests raced from many goroutines at once, which must leave exactly
one friendship row and an answer for every caller.

```sh
# Against your disposable PostgreSQL instance, never a production database:
PLATFORM_TEST_DATABASE_URL=postgres://postgres:local-test-only@127.0.0.1:55443/postgres go test -race ./...
```

Without the variable, `go test -race ./...` runs the unit tests and reports the
database tests as skipped. CI supplies the variable and a dedicated test service.

## Schema

`migrations/` holds the social service's own migrations for the tables these
queries use: friendships, comparison consent, avatars, the profile wall, display
names and read markers. Each has an `up` and a `down` file and is safe to
repeat. They extend an existing `users` table that is defined elsewhere and is
not published, so they are reference material rather than a standalone
database; this package still does not run migrations. Numbering starts at
`000002` because the first migration concerns a credential column.

No database export is included. To integrate this package in another service,
provide a `users` table, apply the migrations, and supply a correctly
authorized calling layer; the temporary test schema is only a test fixture.
