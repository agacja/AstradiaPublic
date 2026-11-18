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
| `frontend/src/App.tsx`, `ChartWheel.tsx`, styles, contracts, contract tests | New public reviewer interface and examples; not a claim to be the complete production UI |
| `packages/social-store/social.go` | Product social persistence functions, with the package name adjusted for independent use |
| `packages/social-store/social_test.go` | Existing query/consent regression test using temporary PostgreSQL tables |
| `packages/social-store/store.go` | Minimal independent pool constructor; no credential vault, trading store, migration runner or production HTTP routes |
| `services/ephemeris/**` | Independently scoped astronomy service, including its source offer, pinned dependencies, data and license notices |
| Documentation and publication checks | Written for this public workspace |

The Go package is a database adapter, not an authorization service. The Rust
astronomy service is an astronomy adapter, not a signal engine. These distinctions
are intentional and are part of the review scope.

Only generic astronomical data is bundled under the astronomy service. That
data is distinct from proprietary market datasets, private calibration tables,
customer birth data, and trading output fixtures, none of which are included.
