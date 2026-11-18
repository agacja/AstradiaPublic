# Public Swiss astronomy service

This independent service accepts a timestamp and optional coordinates and returns ten planet positions and speeds, true node and Placidus cusps. It contains no asset registry, trading interpretations, score weights, strategies, exchange access or wallet credentials. Unsupported polar Placidus houses return null; the client chooses its own fallback.

Rust 1.94+, `cargo build --locked`. Set a random 32+ character `EPHEMERIS_SERVICE_SECRET`; run with `EPHEMERIS_BIND=127.0.0.1:8091`. POST JSON to `/v1/ephemeris` with `x-ephemeris-secret`, e.g. `{"timestampMs":1791676800000,"latitude":51.5074,"longitude":-0.1278}`. This infrastructure key is independent of any trading/API credentials. Use HTTPS or private networking. Credentials are not included in the source archive. Requests are size limited, quota limited and CPU bounded.

`GET /health` reports readiness. `GET /source` offers the exact service source, lockfile, build instructions, notices and ephemeris data embedded at compilation, without authentication. Source dependencies are pinned in Cargo.lock and available from crates.io. Build using `docker build -t ephemeris .` from this directory only. The source archive is built from an explicit file allowlist and this directory's astronomical data; no private application files are included.

This service uses the AGPL-3.0-only license (LICENSE). Swiss Ephemeris copyright Dieter Koch and Alois Treindl / Astrodienst AG; preserve SWISS-LICENSE. `sweph` / `sweph-sys` 0.3.0 are AGPL wrappers. Bundled data originated in swisseph-wasm 0.0.5; its supplied notice is preserved under data/ephemeris/LICENSE. The HTTP DTO file src/protocol.rs is separately MIT licensed under PROTOCOL-LICENSE. This exception does not relicense Swiss or its wrappers.

The published upstream licensing terms are at https://www.astro.com/swisseph/swephinfo_e.htm. Separating processes is a technical boundary, not a ruling on license obligations for a larger deployment. This source offer covers this service; it does not resolve the existing application's separate Swiss WASM integrations.
