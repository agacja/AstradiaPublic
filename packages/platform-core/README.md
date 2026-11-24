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
| `state.rs` | Redis-backed key/value state, owner-checked leases and a monotonic per-signer nonce, each implemented as one atomic Lua script; response and connection timeouts so a silent Redis fails a call instead of holding it forever; an in-memory twin for tests; a lease guard that releases on drop |
| `flight.rs` | Request coalescing: callers that ask for the same thing at the same moment share one piece of work, in async, blocking and short-lived cached forms. No lock is held while work runs, a failure is shared and then forgotten, and a cancelled caller hands the work to one still waiting |
| `signing.rs` | Hyperliquid L1 action hashing (named MessagePack field order), EIP-712 digests for user actions, a local signer, and a remote signer that signs through the provider's `secp256k1_sign` RPC method, authorizes each request with a P-256 signature over canonical JSON, logs only the provider's short refusal code, and verifies the returned signature against the expected wallet |
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
network access is needed: the state tests use the in-memory store and a silent
local socket, the signing tests use fixed vectors and a stand-in provider on a
local port.

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
