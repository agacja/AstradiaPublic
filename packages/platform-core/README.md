# Platform core (Rust)

Coordination and cryptographic primitives taken from Astradia's native Rust API
and trading worker. They are parts of the production binaries that have nothing
to do with deciding a trade: how a stored credential is sealed, how two worker
replicas avoid acting at the same time, how an exchange action is hashed and
signed, and how a payment notification is verified before it can change a
membership.

**No signal evaluation, strategy selection, sizing, order planning or replay
code is in this crate**, and nothing here imports it. The crate builds and tests
on its own, which is the practical proof of that separation.

| Module | What it shows |
| --- | --- |
| `vault.rs` | AES-256-GCM envelope for stored credentials in a versioned binary format, for a BYTEA column; the Go package in `../go-primitives` uses the same format and also opens an older text layout |
| `state.rs` | Redis-backed key/value state, owner-checked leases and a monotonic per-signer nonce, each implemented as one atomic Lua script; batched reads in one round trip, an atomic counter created with its expiry in the same step, queues keyed by id whose entries cost the same however many wait, a delete that happens only while a key still holds what was read, a read-ahead for the reads of one cycle, and a mark that only an operator writes, so that a store which came back empty is never taken for one where nothing is stopped; response and connection timeouts so a silent Redis fails a call instead of holding it forever; an in-memory twin and a Redis stand-in for tests; a lease guard that releases on drop |
| `flight.rs` | Request coalescing: callers that ask for the same thing at the same moment share one piece of work, in async, blocking and cached forms. No lock is held while work runs, a failure is shared and then forgotten, and a cancelled caller hands the work to one still waiting. The cache can keep a result for a lifetime of its own, and when full it drops what has expired and then the value read least recently, never everything at once |
| `signing.rs` | Hyperliquid L1 action hashing (named MessagePack field order), EIP-712 digests for user actions, a local signer on k256 0.14, and a remote signer that signs through the provider's `secp256k1_sign` RPC method, authorizes each request with a P-256 signature (p256 0.14) over canonical JSON, logs only the provider's short refusal code, and verifies the returned signature against the expected wallet. Both curves moved off crypto-bigint 0.5 (RUSTSEC-2026-0387); for every digest below the curve order the tests compare the signatures with the bytes the earlier signers produced |
| `webhooks.rs` | Stripe (HMAC-SHA256, exact bytes, timestamp window) and NOWPayments (HMAC-SHA512 over canonical JSON) notification verification, both failing closed on a missing secret |
| `membership.rs` | The time-boxed subscription check, kept separate from authentication |
| `http.rs` | An HTTP client that never follows redirects, one shared instance of it so callers built per request reuse one connection pool, and a bounded JSON response reader so an upstream cannot force unbounded buffering |

## Tests

```sh
cargo fmt --check
cargo clippy --locked --all-targets -- -D warnings
cargo test --locked
```

Rust 1.94+ is required. No database, Redis instance, exchange account or
network access is needed: the state tests use the in-memory store, a silent
local socket and a Redis stand-in that answers the real wire protocol on a
local port, and the signing tests use fixed vectors and a stand-in provider on a
local port. With `REDIS_TEST_URL` naming a disposable Redis on this machine,
the same store checks also run against a real one.

The crate forbids unsafe code and denies `unwrap` and `expect` outside tests
(`clippy.toml` allows them there); an invariant that cannot fail carries an
`#[expect]` with its reason. Release builds check integer overflow.

`tests/signing_parity.rs` checks the hashing and signing path against golden
vectors: the same action must produce the same hash and the same `r`, `s`, `v`.
The published test checks this Rust implementation; the earlier Go and
TypeScript implementations are held to the same vectors privately.
The vault test decrypts the fixture's binary blobs, in the layout the Go
package writes, with the same synthetic key, and
`tests/state.rs` covers lease exclusivity, renewal, expiry and nonce
monotonicity against the in-memory store.

## Fixtures

Everything under `tests/fixtures/` is synthetic. The signing vectors use the
well-known test key `0x0123…0123`; the vault fixture uses the key `000102…1e1f`. The vectors are a small generic
subset of the shared set: one exchange-signed action and one user-signed
action, on each network, with made-up values.

## What is not here

Request authentication and rate limiting, the HTTP routes, database queries,
billing flows, the exchange adapter and the worker loop that call these
primitives stay private, together with the evaluation engine. Key prefixes are
visible in this source; their protection is the secrets and network isolation
supplied at deployment, never obscurity.
