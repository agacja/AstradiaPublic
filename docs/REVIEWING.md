# Reviewer guide

Start with the README scope table and architecture diagram, then run the frontend.
Every demo interaction stays local. There is no payment, account creation, wallet
connection, exchange submission, or production API call.

Suggested review order:

1. `frontend/src/components/Glyph.tsx` and `ChartWheel.tsx`: reusable rendering and
   separation between display data and calculations.
2. `frontend/src/utils/birthData.ts`: account changes clear unowned or foreign
   cached personal data.
3. `frontend/src/utils/payment-access-watch.ts`: bounded polling, visibility/focus
   refresh, account isolation, cleanup and transient-error handling.
4. `frontend/src/contracts.ts`: narrow output parsing and refusal to consume
   extra internal evaluation fields.
5. `packages/social-store/social.go`: parameterized queries, relationship
   visibility and persisted consent; note the required calling-layer authorization.
6. `services/ephemeris/src/main.rs`: validated requests, independent credentials,
   bounded expensive work and a component-scoped source offer.
7. `scripts/check-public-boundary.py`: the reviewed file manifest and publication
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

The private product has additional tests and deployment infrastructure which
are not reproduced here. Public checks do not validate the private algorithm's
investment performance or replace private execution-system tests.
