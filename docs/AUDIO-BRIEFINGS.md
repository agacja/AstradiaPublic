# Daily audio briefings

The production Rust API serves paid, authenticated daily briefings from the member's personal reading and saved birth profile. The React player appears in Your Chart for Pro members. Free members see an upgrade card.

## Configuration and deployment

Set server-only `ELEVENLABS_API_KEY`, `ELEVENLABS_VOICE_ID` and `ELEVENLABS_MODEL_ID` on the API service. The voice and model are deployment configuration. No credentials are included in the frontend. The Rust API creates only its additive `audio_briefings` table and index on startup under a PostgreSQL advisory lock. The migration is safe to repeat; existing billing, user and trading data is unchanged. No Node backend is used.

## API and access

- `GET /api/briefings/daily`: today's transcript and generation status.
- `POST /api/briefings/daily`: create or reuse today's narration; only an empty request body or `{}` is accepted.
- `GET /api/briefings/daily/audio?id=...`: authenticated, account-scoped MP3 playback.

Every endpoint rechecks current paid membership, including cached downloads. Worker credentials cannot access personal audio. Date, account, transcript and voice are selected server-side. The transcript, date, birth profile, voice, model and script version determine cache identity. A profile change invalidates old audio.

## Cost and reliability

Generation is on demand, and a cached recording survives restarts and costs
nothing further to replay. An atomic database claim prevents duplicate
generation across replicas, and a short lease recovers work interrupted by a
restart. Attempts per account per day are bounded, failures count against that
bound, and a changed profile does not reset it. A disconnected browser does not
cancel caching an in-flight paid request, and there is no automatic paid retry.

The provider request is bounded in time, text length and response size. Only
the narrated text and voice settings are sent to the provider, never raw birth
details or account credentials. Provider error bodies are not returned or
logged. Audio responses are private and never publicly cached. Old audio is
removed after a short retention period; deleting an account removes its audio.

## Validation

The private API's database integration tests use only `PLATFORM_TEST_DATABASE_URL` on loopback and a nonstandard port, with disposable isolated schemas. Their speech provider is a local mock; tests spend no ElevenLabs credits.

## What is public

The React player is `frontend/src/components/DailyAudioBriefing.tsx` with its client in `frontend/src/utils/audio-briefing-api.ts`, and the table is `docs/schema/audio-briefings.sql`. The API route, the claim and lease queries, and the module that writes the reading stay private.
