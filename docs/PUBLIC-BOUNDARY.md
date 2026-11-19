# Public/private publication boundary

The public repository is a curated snapshot, not a filtered view of private Git
history. A file being frontend code does not automatically make it suitable for
publication: client helpers can contain business rules, and fixtures can reveal
model behavior even when the implementation itself is absent.

## Approved categories

- General presentation components and visual geometry.
- Components that read only public market data, and players for content the
  server has already authorized, with no scoring or interpretation inside them.
- Account-scoped browser storage and membership synchronization utilities.
- A free-feature presentation policy; server authorization remains separate.
- Generic astronomy infrastructure, with its existing licenses and notices.
- Social persistence and consent-query code, with synthetic test data, and the
  definitions of the tables it uses.
- Platform security and coordination primitives that contain no evaluation or
  execution logic: the credential envelope, distributed leases and nonces,
  exchange-protocol hashing and signing, payment-notification verification and
  the membership time check.
  Their keys, vectors and fixtures are synthetic.
- Illustrative output contracts and high-level architectural documentation.

## Excluded categories

- Scoring equations, feature transformations and weighted aggregations.
- Parameter tables, calibration coefficients, asset-specific interpretations,
  action thresholds, risk/sizing policies, and strategy selection internals.
- Current or historical implementations of these systems in any language.
- Trading workers, order planning, the exchange adapter, execution journals and
  replay engines. Protocol signing and the credential envelope are published as
  primitives; the code that decides what to sign, and the wiring that loads real
  key material, are not. Request authentication and rate limiting are described,
  not published.
- Anything that would help an outsider tell the product's exchange activity
  apart from anyone else's.
- Modules named as presentation code that nonetheless contain evaluation logic
  or interpretation text.
- Model parity fixtures, calibration datasets, research notebooks, detailed
  backtest exports and precomputed trading outputs.
- Production configuration, private environment files, user/payment records,
  logs, backups, database exports, credentials and private compiled artifacts.

## Update procedure

1. Work in this repository's independent checkout. Do not add a private repo as
   an upstream and do not merge its history.
2. Review each proposed source file and its complete import/dependency closure.
   Check comments, examples, generated files, logs and test fixtures as well.
3. Run tests and the production frontend build.
4. Inspect `git diff --cached`. Update `public-files.json` **only after review**:
   it is an exact file-and-hash manifest, not an automatic sync list.
5. Run `python3 scripts/check-public-boundary.py` and a dedicated secret scan
   (`gitleaks dir --redact .`). Scan new Git history as well (`gitleaks git --redact .`).
6. Push only to `agacja/AstradiaPublic`. The optional local pre-push hook checks
   that destination and runs the boundary check. Ask the owner to review changes
   to the approved manifest or publication checks.

The manifest excludes itself to avoid a recursive hash; review its changes
directly. It includes the publication checker and CI workflow so changes to
those controls are visible in the same review. CI checks the public state after
push; **it cannot undo a disclosure already pushed to a public repository**.
Local inspection and scanning before publication are required.

No checker can prove that arbitrary new code contains no proprietary ideas.
The protection is conservative selection, fresh history, explicit scope, and
human review supported by automated checks. There is deliberately no automatic
private-to-public synchronization job.
