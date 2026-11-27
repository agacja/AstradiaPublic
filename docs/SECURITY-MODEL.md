# Security model

How identity, payments, stored credentials and consent are handled in
production, and where the published code that implements each piece lives. For
the system overview see [ARCHITECTURE.md](ARCHITECTURE.md); for the trading
worker see [EXECUTION-SAFETY.md](EXECUTION-SAFETY.md). To report a
vulnerability, use the process in the root [SECURITY.md](../SECURITY.md).

The design rule throughout is that protection comes from secrets supplied at
deployment and from checks that fail closed, never from the source being
hidden. That is why the primitives can be published. This document describes
the source as deployed in November 2026; it is not a statement about any
deployment's network configuration.

## Trust boundaries

```mermaid
flowchart LR
    subgraph Z0["Untrusted: internet"]
        B["Browser"]
        PP["Payment providers"]
    end

    subgraph Z1["Platform"]
        API["Rust API"]
        W["Rust worker"]
        S["Go social service"]
        A["Astronomy service"]
    end

    subgraph Z2["Third parties"]
        PR["Privy"]
        HL["Hyperliquid"]
    end

    B -->|"(1) session token"| API
    PP -->|"(2) signed notification"| API
    W -->|"(3) service identity"| API
    API -->|"(4) shared secret"| A
    API -->|"(5) forwarded member token"| S
    W -->|"(6) authorized digest request"| PR
    API -->|"(6) authorized digest request"| PR
    W -->|"(7) signed action"| HL
    API -->|"(7) signed action"| HL

    classDef pub fill:#e6f4ea,stroke:#1e7e34,color:#0b3d17
    classDef part fill:#e3f0fb,stroke:#1565c0,color:#0d3c6e
    classDef ext fill:#f4f4f4,stroke:#777777,color:#222222
    class A pub
    class API,W,S part
    class B,PP,PR,HL ext
```

| # | Crossing | What is checked | Published in |
| --- | --- | --- | --- |
| 1 | Browser to API | Signed, expiring session token; a single `Authorization` header; a per-member quota. A route that needs no session answers every caller from one shared, periodically refreshed calculation, or is limited per network and against a budget shared by all such callers. Every JSON body and query string the API parses for a member's request may carry only the fields its route declares | Described only |
| 2 | Provider to API | HMAC over the exact request bytes or canonical JSON, with a timestamp window where the provider supplies one | `platform-core/src/webhooks.rs` |
| 3 | Worker to API | A service identity distinct from any member; an invalid one is rejected outright and never falls back to a member identity | Described only |
| 4 | API to astronomy | Independent shared secret, size limits, quota and bounded CPU work | `services/ephemeris/src/main.rs` |
| 5 | API to social | The member's own token is forwarded and validated again by the social service | Described only |
| 6 | Platform to Privy | Each signing request is authorized by a P-256 signature over its canonical JSON and expires quickly | `platform-core/src/signing.rs` |
| 7 | Platform to exchange | Action hash and EIP-712 digest are built locally; a signature obtained from the remote signer is recovered and compared with the expected wallet before submission | `platform-core/src/signing.rs` |

The API signs and submits for actions a member initiates in the app; the worker
does so for automated trading. Both use the same published signing code.

The backend origin a service is configured with must be HTTPS or a
private-network host and may not embed credentials, and outbound clients do not
follow redirects.

## Identity and sessions

The diagram shows the Privy sign-in path, which is the one the product
presents. Privy establishes who the person is; the platform decides what that
identity may do.

```mermaid
sequenceDiagram
    autonumber
    actor M as Member
    participant UI as React client
    participant P as Privy
    participant API as Rust API
    participant DB as PostgreSQL

    M->>UI: sign in
    UI->>P: authenticate
    P-->>UI: access token
    UI->>API: access token, claimed email and wallet
    API->>API: verify the token against Privy's key
    API->>P: read the identity's linked accounts
    P-->>API: linked accounts
    API->>API: keep only claims that match a linked account
    API->>DB: find or create the account for this identity
    DB-->>API: account
    API-->>UI: platform session token
    UI->>API: later requests carry the session token
```

- A claimed email or wallet is used only if Privy lists it as linked to that
  identity. The browser cannot assert its way into another account.
- An account already bound to one identity is never rebound to another.
- Membership is checked separately from authentication on every paid route,
  including downloads of content that was generated earlier
  (`platform-core/src/membership.rs`).
- Every password hash the platform writes is Argon2id; an older bcrypt hash is
  replaced at sign-in wherever bcrypt can confirm the whole password.

## Stored credentials and signing

What is published here is the format a stored credential is sealed in, and the
check applied to a signature before it is used. How credentials are provisioned
and moved between services is private and is not described.

```mermaid
flowchart TB
    subgraph REST["At rest"]
        E1["AES-256-GCM envelope:<br/>version byte, 96-bit nonce,<br/>ciphertext and tag"]
        E2["Stored in a PostgreSQL BYTEA column"]
        E3["Opened into a type that redacts<br/>itself when printed or serialized"]
        E1 --> E2 --> E3
    end

    subgraph SIGN["Remote signing"]
        R1["Canonical JSON of the request"]
        R2["Authorized with a P-256 signature,<br/>short expiry"]
        R3["Privy returns a signature"]
        R4["Recover the signer locally"]
        R5{"Equals the expected wallet?"}
        R1 --> R2 --> R3 --> R4 --> R5
        R5 -->|"yes"| OK["Used"]
        R5 -->|"no"| NO["Rejected, nothing sent"]
    end

    classDef pub fill:#e6f4ea,stroke:#1e7e34,color:#0b3d17
    classDef ext fill:#f4f4f4,stroke:#777777,color:#222222
    class E1,E2,E3,R1,R2,R4,R5,OK,NO pub
    class R3 ext
```

The envelope exists in two languages (`platform-core/src/vault.rs`,
`go-primitives/vault`) tested with the same synthetic key and plaintext. The
Go package also opens the older text layout, with either IV length the layout
was written with; the Rust envelope reads the binary layout only. In Go the decrypted value is a type whose `String`, `GoString`
and `MarshalJSON` all return a redaction marker, so it cannot reach a log line
or a JSON encoder by accident; in Rust it is a `SecretString`. Every use is an
explicit, greppable reveal.

The signature check matters most for the remote signer: a response from outside
the process is never trusted because of where it came from.

Local keys sign with k256 0.14 and each request to the signing provider is
authorized with p256 0.14, both on crypto-bigint 0.7: the 0.5 line under the
earlier signers had secret-dependent timing (RUSTSEC-2026-0387). The new
signers were compared with the old ones before the switch: for every digest
below the curve order the new signatures are byte for byte the earlier ones,
over fixed keys, actions and requests. Above it, which a Keccak digest
practically never is and about one SHA-256 request hash in 2^32 is, they
differ and still verify.

## Payments and membership

```mermaid
sequenceDiagram
    autonumber
    actor M as Member
    participant UI as React client
    participant API as Rust API
    participant PV as Payment provider
    participant DB as PostgreSQL

    M->>UI: choose a plan
    UI->>API: create checkout
    API->>PV: create session or invoice
    PV-->>API: checkout address
    API-->>UI: checkout address
    M->>PV: pay on the provider's page
    PV->>API: signed notification
    API->>API: verify signature before any database write
    alt signature invalid, stale or secret missing
        API-->>PV: rejected
    else verified
        API->>DB: lock payment row, complete once, update membership
        API-->>PV: accepted
    end
    loop on focus, visibility change and a bounded timer
        UI->>API: refresh membership
    end
    API-->>UI: current membership
```

- Membership changes only on the server: from a verified provider notification,
  or from the server's own authenticated lookup with the provider when a member
  asks it to recover a payment. A return URL or a local UI switch cannot grant
  access.
- Completing a payment takes a row lock, so a repeated or concurrent
  notification completes it once.
- The Stripe check uses the exact received bytes and rejects a timestamp
  outside a short window. The NOWPayments check canonicalizes the JSON first,
  so key order cannot be used to forge or break a signature. Both return false
  when the secret is empty.
- The browser side is `frontend/src/utils/payment-access-watch.ts`: bounded
  polling, refresh on focus and visibility, and cleanup when the account
  changes. A renewal counts as arrived only when the membership's end moves
  later, and a member who already has access is looked up only while a
  payment they started is awaited, so returning to the page does not reload
  their account.

## Hardening around the request path

- Every JSON body and query string the API parses for a member's request may
  carry only the fields its route declares; anything else is refused. The
  social service refuses an unexpected body field too. Provider notifications
  are verified as received instead (see above).
- Requests to the API run under a deadline, apart from generating an audio
  briefing, which waits on the voice provider under a bound of its own. In the
  API a handler that panics answers with an error instead of dropping the
  connection, and the API and the worker log a panic by its place in the code,
  never by its message.
- The published Rust crates forbid unsafe code, deny `unwrap` and `expect`
  outside tests, and check integer overflow in release builds.
- Rust dependencies are checked for published advisories, licenses and sources
  (cargo-audit, cargo-deny), Go modules for known vulnerabilities in the
  code they call (govulncheck), and the frontend's locked packages for
  advisories and valid registry signatures (`npm audit`)
  ([architecture, section 9](ARCHITECTURE.md#9-delivery-and-quality-gates)).

## Fail-closed properties and the tests that pin them

Request authentication and rate limiting have their own private tests. The
properties below are the ones whose code and tests are published.

| Property | Test in `packages/platform-core` |
| --- | --- |
| A tampered ciphertext, a wrong version byte or a short blob does not decrypt | `vault::tests::v1_roundtrip_and_tampering` |
| A blob in the layout the Go package writes, under the same synthetic key, decrypts here | `vault::compatibility::binary_blobs_in_the_go_package_layout_decrypt` |
| A notification with a changed body, a stale timestamp or an empty secret is rejected | `webhooks::tests::stripe_requires_exact_bytes_and_fresh_timestamp` |
| Key order cannot change a notification's signature | `webhooks::tests::now_signature_is_order_independent_and_fails_closed` |
| An inactive, expired or unparseable subscription is not a subscription | `membership::tests::subscription_requires_active_status_and_unexpired_valid_date` |
| An action hashes and signs to the bytes recorded in the golden vectors | `signing_parity::matches_all_existing_sdk_golden_vectors` |
| A remote signer's refusal, or a signature made by any wallet but the expected one, never becomes a signature | `signing::tests::a_refusal_or_a_signature_by_another_wallet_never_becomes_a_signature` |
| Only the signing provider's short refusal code is logged, never its message | `signing::tests::only_the_providers_short_code_is_taken_from_a_refusal` |
| A lease is exclusive, owner-checked and renewable only by its owner | `state::lease_is_exclusive_until_released_and_owner_checked`, `state::same_owner_renews_and_expired_lease_can_be_taken` |
| A nonce never repeats when the clock stalls or steps back | `state::nonce_never_repeats_when_the_clock_stalls_or_goes_back` |
| A store that came back empty is never taken for an armed one: a missing, damaged or unreadable mark is no mark, the mark has no expiry, and arming never rewrites one in force | `state::armed::tests::only_a_true_mark_is_the_mark`, `state::armed::tests::arming_writes_the_mark_once_and_never_rewrites_it`, `state::tests::the_mark_is_kept_in_redis_without_an_expiry_and_goes_with_the_state` |
| A value that cannot be read fails its own key in a batched read, never the values read beside it | `state::tests::several_values_are_read_in_one_round_trip_and_fail_one_by_one`, `state::tests::a_value_that_is_not_text_fails_its_own_key_and_no_other` |
| Callers adding to one counter at once each get a sum of their own, so a limit kept in it cannot be raced past; a new count gets its expiry in the same step | `state::tests::the_in_memory_store_keeps_the_contract`, `state::tests::the_real_redis_checks_hold_on_the_stand_in`, `state::tests::a_count_is_created_with_its_expiry_in_the_same_step` |
| The current signers produce the earlier signers' bytes, and a digest at or above the curve order still gets a valid signature | `signing::tests::action_signatures_are_the_bytes_ethers_produced`, `signing::tests::authorization_signatures_are_the_bytes_p256_0_13_produced`, `signing::tests::a_digest_at_or_above_the_order_still_gets_a_valid_signature` |
| A Redis that stops answering fails the call instead of holding it forever | `state::timeout_tests::connecting_to_a_silent_server_fails_instead_of_hanging`, `state::timeout_tests::a_command_that_is_never_answered_fails_instead_of_hanging` |
| A Redis that shut down is reached again within seconds of its return, by a store that was running and by one that was starting | `state::outage_tests::a_store_reads_again_within_seconds_of_the_servers_return`, `state::outage_tests::a_store_started_while_the_server_is_away_connects_within_seconds_of_its_return` |

Four frontend modules read data from outside and check it where it enters.
The first two were written for this workspace; the other two are product
modules:

| Property | Test in `frontend/tests` |
| --- | --- |
| A signal summary with an extra field, a field of the wrong shape, or an unavailable result with any action but `WAIT` is refused, always with the same message, and what is returned is built from checked values only | `contracts.test.ts` |
| A candle response that is not a list of candles, or has a price that is not a number, is refused before the chart reads it; the request carries no credential and fails when it is not answered in time | `hyperliquid-candles.test.ts` |
| An answer from the briefing routes that is not JSON, or a briefing with a field missing, becomes a message a member can read, never a parser error, and of a failure only words the server sent are shown | `briefing-response.test.ts` |
| A message of the live candle stream with a field that is missing or not a number is dropped, so the chart and the price are never drawn from NaN | `candle-message.test.ts` |

## Social consent

Friendship and permission to compare charts are separate decisions with
separate state. The diagrams show the main transitions.

```mermaid
stateDiagram-v2
    direction LR
    state "Friendship" as F {
        [*] --> None
        None --> Pending: request
        Pending --> Accepted: addressee accepts
        Pending --> Declined: addressee declines
        Accepted --> None: either side removes
        Declined --> None: either side removes
    }
```

```mermaid
stateDiagram-v2
    direction LR
    state "Chart comparison consent, one row per pair" as C {
        [*] --> NoConsent
        NoConsent --> Requested: one side asks
        Requested --> Granted: other side accepts
        Requested --> Granted: other side also asks
        Requested --> Refused: other side declines
        Refused --> Requested: a fresh request reopens it
        Granted --> NoConsent: either side revokes, or the friendship ends
    }
```

- A comparison runs only when the pair's consent is granted. Friendship alone
  is never enough.
- A declined comparison request is not revealed to the person who asked.
- Ending a friendship deletes the pair's consent in the same transaction, so a
  stale grant cannot silently unlock comparison again after re-friending.
  Either side can also revoke consent on its own at any time.
- Display names replace emails on every social surface; a member's email is
  never shown to another member.
- Realized profit and loss is never shown to a friend, whatever a stored
  display preference says.

The persistence for both state machines is published in
`packages/social-store`. The comparison itself and the HTTP layer are not.
