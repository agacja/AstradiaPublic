# Go primitives

Two dependency-free packages from Astradia's Go social service. Both use only
the standard library, so there is no `go.sum`.

| Package | What it shows |
| --- | --- |
| `vault` | AES-256-GCM envelope for stored credentials in a versioned binary layout; a `Secret` type whose `String`, `GoString` and `MarshalJSON` all redact, so a decrypted value cannot reach a log line or a JSON response by accident and every real use is a greppable `Reveal()` call; read-compatibility with the earlier text format, with either IV length it was written with; a cross-runtime vector proving Go reads what the TypeScript runtime wrote |
| `ephem` | Geocentric ecliptic longitudes for ten bodies in pure Go: JPL approximate Keplerian elements, a Newton solver for Kepler's equation, a truncated Meeus lunar series and precession to the equinox of date, checked against Swiss Ephemeris reference values |

`ephem` returns positions only. It has no aspects, orbs, weights, scores or
interpretations, and nothing in this module knows what an asset or a trade is.
The Rust crate in `../platform-core` contains the same vault format. Both are
tested with the same key and plaintext; this package also reads ciphertext
produced by the earlier TypeScript runtime.

## Tests

```sh
gofmt -l .        # prints nothing
go vet ./...
go test -race ./...
```

Go 1.26.11 or newer is required. No database or network access is needed. All keys and
plaintexts in the tests are synthetic.
