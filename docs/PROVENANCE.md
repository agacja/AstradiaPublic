# Component provenance

Prepared from Astradia's maintained source in November 2026. No private commit
history, production configuration, account data, or model reference results were
copied. The public repository's initial commit is its own root commit.

| Public files | Origin and changes |
| --- | --- |
| `frontend/src/components/Glyph.tsx` | Product SVG glyph module; presentation paths only |
| `frontend/src/utils/feature-access.ts` | Product free-feature presentation utility |
| `frontend/src/utils/payment-access-watch.ts` | Product browser membership watcher; injected status/refresh functions, no credentials |
| `frontend/src/utils/birthData.ts` | Product account-isolated cache; type import redirected to an isolated DTO file |
| `frontend/src/utils/chart-types.ts` | Display-only message types extracted from a mixed private calculation module; implementation excluded |
| `frontend/tests/free-tier.test.ts`, `payment-access-watch.test.ts` | Existing regression tests using synthetic local fixtures |
| `frontend/src/components/HyperliquidChart.tsx`, `styles/realtime-chart.css` | Product live price chart and its stylesheet; two import paths redirected and one comment naming an internal design file removed. Reads public market data only |
| `frontend/src/lib/hyperliquid-candles.ts` | Written for this workspace: a read-only candle fetch standing in for the product's exchange SDK client |
| `frontend/src/components/DailyAudioBriefing.tsx`, `utils/audio-briefing-api.ts`, `styles/audio-briefing.css` | Product audio briefing player, client and stylesheet; three import paths redirected |
| `frontend/src/utils/astrocartography.ts`, `frontend/tests/astrocartography.test.ts` | The geometry half of the product module: equatorial conversion, sidereal time and the four line families, with the Julian-day helper inlined. The interpretation text that follows it in the product file is excluded. The test was written for this workspace |
| `frontend/src/utils/session.ts` | Written for this workspace: the two session helpers the audio player imports, without the private API client around them |
| `frontend/vite.config.ts` | Reviewer build configuration carrying the product's bundle guard plugin unchanged |
| `docs/AUDIO-BRIEFINGS.md`, `docs/schema/audio-briefings.sql` | Product design note with the voice identifier, model, service name, private paths and every numeric limit removed, and a closing section added; the table definition is unchanged |
| `frontend/src/App.tsx`, `ChartWheel.tsx`, styles, contracts, contract tests | New public reviewer interface and examples; not a claim to be the complete production UI |
| `packages/social-store/social.go` | Product social persistence functions, with the package name adjusted for independent use |
| `packages/social-store/social_test.go` | Existing query/consent regression test using temporary PostgreSQL tables |
| `packages/social-store/migrations/*` | The social service's migrations for the social tables, unchanged apart from one reworded comment. The first migration, which concerns a credential column, is not included |
| `packages/social-store/store.go` | Minimal independent pool constructor; no credential vault, trading store, migration runner or production HTTP routes |
| `services/ephemeris/**` | Independently scoped astronomy service, including its source offer, pinned dependencies, data and license notices |
| `packages/platform-core/src/vault.rs`, `state.rs` | Production Rust modules, byte-identical to the source deployed in November 2026 |
| `packages/platform-core/src/signing.rs` | Production Rust module; two paths changed so its HTTP helpers resolve inside this crate, and two account-setup action types omitted |
| `packages/platform-core/src/http.rs` | The bounded response reader lifted from the private exchange adapter and the no-redirect client lifted from the private request-security module, given their own error type; nothing else from either |
| `packages/platform-core/src/webhooks.rs` | The two notification-verification functions and their unit tests, lifted unchanged from the private billing module; checkout, pricing and membership writes excluded |
| `packages/platform-core/src/membership.rs` | The subscription time check, its timestamp parser and its table test, lifted from the private access and database modules; the route policy and queries excluded |
| `packages/platform-core/tests/signing_parity.rs`, `tests/fixtures/*` | The existing golden-vector test with the crate import and fixture path changed and a second, SDK-comparison test removed. The vectors are a small subset of the shared set (one exchange-signed and one user-signed action per network); keys and values are synthetic |
| `packages/platform-core/tests/state.rs`, `Cargo.toml`, `lib.rs` | Written for this public workspace; `Cargo.lock` is the production lockfile reduced to this crate's dependencies |
| `packages/go-primitives/vault/*` | Production Go package and tests, byte-identical to the source deployed in November 2026 |
| `packages/go-primitives/ephem/*` | Production Go package and test, formatted with `gofmt`; three comments reworded to describe the current runtime and to match what the test checks |
| `docs/ARCHITECTURE.md`, `SECURITY-MODEL.md`, `EXECUTION-SAFETY.md` | Written for this public workspace from the source deployed in November 2026 and reviewed against it, with policy values omitted. They describe that source, not a live deployment's configuration |
| Documentation and publication checks | Written for this public workspace |

The Go package is a database adapter, not an authorization service. The Rust
astronomy service is an astronomy adapter, not a signal engine. The platform
core is a set of primitives, not the API or the worker that call them. These
distinctions are intentional and are part of the review scope.

Only generic astronomical data is bundled under the astronomy service. That
data is distinct from proprietary market datasets, private calibration tables,
customer birth data, and trading output fixtures, none of which are included.
