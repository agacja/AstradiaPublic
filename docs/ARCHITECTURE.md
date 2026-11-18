# Architecture

## Production and this repository

Astradia's production application is larger than this public workspace.
The diagram in the root README describes production responsibilities; it does
not imply that the complete production system can be started from this repo.

| Component | Production responsibility | Public scope |
| --- | --- | --- |
| React client | Product navigation, chart and result presentation, authenticated API calls | Reusable modules and a standalone reviewer UI |
| Rust API | Authentication, membership enforcement, user data, billing, analysis, and social proxying | Architectural description only |
| Shared native engine | Proprietary feature evaluation, calibrated aggregation, and final signal decisions | No implementation, parameters, reference outputs, or binaries |
| Rust worker | Account coordination, private risk policy, order lifecycle and reconciliation | Architectural description only |
| Astronomy service | Generic planetary positions and house geometry | Independent service source and existing license/data notices |
| Go social service | Profiles, friendships, consent-aware comparisons, and related product features | Social database package only; calculation and HTTP layers excluded |
| PostgreSQL | Durable accounts, memberships, payments, preferences, and relationships | No database dump; only synthetic test schemas |
| Redis | Shared state, coordination, leases and caches | No snapshot or deployment configuration |

## Signal flow

1. The client requests product data using its authenticated session.
2. The Rust API checks the user's identity, membership, and requested scope.
3. The private engine combines generic astronomical inputs with its private
   evaluation logic and internal data. The math, model parameters, and decision
   rules are not part of any public package.
4. A response adapter selects the fields needed by the client. Internal evidence
   and configuration are not an API contract.
5. The worker uses the same private evaluation library, then applies its own
   private account/execution checks before interacting with an exchange.

The public `frontend/src/contracts.ts` is an **illustrative narrow boundary**,
not a promise that every production endpoint uses that exact schema. Its parser
rejects unexpected fields; the demo leaves the signal unavailable. It never
substitutes a locally re-created trading algorithm.

The published astronomy component is a separate HTTP process. It is also used
by the private platform, but it knows only time, coordinates, positions, speeds,
and houses. It does not know assets, positions, portfolio balances, or strategy
decisions. Process separation describes the implementation and is not a legal
conclusion about licensing obligations of a larger deployment. Existing
component licenses are preserved.

## Identity, membership, and social consent

Privy supplies production identity and wallet integration. Application sessions
and authorization remain server responsibilities. No production provider keys
or application IDs are required by the reviewer UI.

For payments, the production server verifies provider callbacks, binds them to
the correct account and invoice, and commits membership changes idempotently.
The browser's membership watcher only refreshes authenticated state; a return
URL or a local UI switch cannot grant server access. Payment processing and
credentials are not published in this workspace.

Free access and social privacy are separate concerns. Friendship does not imply
permission to compare personal charts. Mutual consent must be checked by the
calling API. The public Go package shows how relationships and consent state
are persisted and read. It does not expose an unauthenticated HTTP interface.

## Storage

PostgreSQL uses typed relational columns for identities, timestamps and other
queryable fields; JSONB is used for document-style preferences. BYTEA is used
where the data is binary, such as avatar images and encrypted credentials in
the private platform. BYTEA is a PostgreSQL type, not a Rust- or Go-specific
database. No private credentials or encrypted credential blobs are published.

## What the reviewer edition does

The frontend can be built on its own. Its chart fixture is deliberately synthetic
and its social controls update local component state only. The Go package and
Rust astronomy service can be tested independently. They are not wired into a
replacement for the hosted product, and there is no automatic connection to
Astradia's production accounts or databases.
