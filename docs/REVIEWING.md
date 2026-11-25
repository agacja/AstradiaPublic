# Reviewer guide

Start with the README scope table and the diagrams in
[ARCHITECTURE.md](ARCHITECTURE.md), [SECURITY-MODEL.md](SECURITY-MODEL.md) and
[EXECUTION-SAFETY.md](EXECUTION-SAFETY.md), then run the frontend.
Every demo interaction stays local. There is no payment, account creation, wallet
connection, exchange submission, or production API call.

Suggested review order:

1. `frontend/src/components/Glyph.tsx` and `ChartWheel.tsx`: reusable rendering and
   separation between display data and calculations.
2. `frontend/src/utils/birthData.ts`: account changes clear unowned or foreign
   cached personal data.
3. `frontend/src/utils/payment-access-watch.ts`: bounded polling, visibility/focus
   refresh, account isolation, cleanup and transient-error handling; a renewal
   is recognised by the membership's end moving later, and a paid-up member is
   not looked up on every return to the page.
4. `frontend/src/contracts.ts`: narrow output parsing and refusal to consume
   extra internal evaluation fields.
   `frontend/src/components/HyperliquidChart.tsx`: chart and WebSocket
   lifecycle. Each run of the chart effect owns a session that its history
   request, socket and reconnect timer check before acting, so a slow response
   or a late reconnect from the previous interval cannot write into the chart
   that replaced it; a stream that keeps dropping is retried less and less
   often (`frontend/src/utils/reconnect-delay.ts`). `frontend/src/components/DailyAudioBriefing.tsx`:
   abortable requests with timeouts, object-URL cleanup, and a remount key so
   one account's audio cannot linger for the next. Both are product modules
   that type-check and build here but are not mounted in the reviewer UI, so
   the demo still makes no network calls; `docs/AUDIO-BRIEFINGS.md` describes
   the server side of the second one.
   `frontend/src/utils/astrocartography.ts`: the spherical geometry behind the
   map lines, with a test of its invariants.
5. `packages/social-store/social.go`: parameterized queries, relationship
   visibility and persisted consent; note the required calling-layer authorization.
   Requests are serialized per pair and per sender with transaction-scoped
   advisory locks, so a double click or two crossed requests have one outcome;
   `social_db_test.go` races them against a real database. A friend's
   dashboard, a comparison pair and the unread counts are each one statement;
   for the dashboard and the pair the friendship is part of the statement's
   condition rather than a check made after it. The database's collation, not
   Go's byte order, decides which member of a pair comes first (`consentPair`,
   tested in an ICU-collated database by `reads_db_test.go`). `indexes.go` builds its one
   index outside the migrations, one instance at a time.
6. `services/ephemeris/src/main.rs`: validated requests, independent credentials,
   bounded expensive work (a burst waits for a calculation slot instead of being
   refused), a graceful stop on SIGTERM and a component-scoped source offer.
7. `packages/platform-core/src/state.rs` and `tests/state.rs`: atomic leases and
   nonces, the guard that releases on drop, and Redis calls that fail instead of
   waiting forever; batched reads, an atomic counter and queues keyed by id,
   checked against a Redis stand-in that answers the real wire protocol.
   `packages/platform-core/src/flight.rs`: request coalescing,
   where callers asking for the same thing at the same moment share one piece
   of work, with cancellation handled so an abandoned key never stays behind,
   and a cache that, when full, drops the value read least recently.
8. `packages/platform-core/src/signing.rs` and `tests/signing_parity.rs`:
   byte-exact protocol hashing, and local verification of a remote signature.
   Its tests run the remote signer against a stand-in provider that checks the
   request's authorization signature the way the real one does, and compare
   the k256/p256 0.14 signers with the bytes the earlier ones produced.
9. `packages/platform-core/src/vault.rs` and `packages/go-primitives/vault`: one
   envelope format in two languages; the Go package also opens the earlier
   text layout, with both IV lengths it was written with.
10. `packages/platform-core/src/webhooks.rs`: payment-notification verification
    that fails closed, with a test for each provider.
11. `scripts/check-public-boundary.py`: the reviewed file manifest and publication
    controls; review the manifest itself rather than trusting its existence.

## Validation

```sh
python3 scripts/check-public-boundary.py
cd frontend
npm ci
npm test
npm run build
```

From the repo root:

```sh
cd packages/social-store
go test -race ./...
```

The social integration test prints a skip without a disposable PostgreSQL
connection. Run it with a loopback address and a nonstandard port to exercise
the database path; see the package README. CI provisions this test database.

```sh
cd services/ephemeris
cargo fmt --check
cargo test --locked
cargo clippy --locked --all-targets -- -D warnings
```

The astronomy crate currently has no embedded Rust unit tests. The CI smoke test
starts the real service and checks health, authorization, valid astronomical
responses, rejected inputs, and the contents of the offered source archive.

```sh
cd packages/platform-core
cargo fmt --check
cargo clippy --locked --all-targets -- -D warnings
cargo test --locked
```

```sh
cd packages/go-primitives
gofmt -l .
go vet ./...
go test -race ./...
```

Neither of these needs a database, Redis, an exchange account or network access.

The private product has additional tests and deployment infrastructure which
are not reproduced here. Public checks do not validate the private algorithm's
investment performance or replace private execution-system tests.
