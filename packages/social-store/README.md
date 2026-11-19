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

Use Go 1.25+ and PostgreSQL 16. The integration test creates session-local
temporary tables and uses synthetic identifiers. It refuses a non-loopback
database or the usual production port.

```sh
# Against your disposable PostgreSQL instance, never a production database:
PLATFORM_TEST_DATABASE_URL=postgres://postgres:local-test-only@127.0.0.1:55443/postgres go test -race ./...
```

Without the variable, `go test -race ./...` compiles the package and reports the
database test as skipped. CI supplies the variable and a dedicated test service.

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
