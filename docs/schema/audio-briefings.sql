-- The cache key is the raw 32-byte SHA-256 digest, stored as BYTEA.
CREATE TABLE IF NOT EXISTS audio_briefings_v2 (
    user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    briefing_date DATE NOT NULL,
    cache_key BYTEA NOT NULL CHECK (octet_length(cache_key) = 32),
    transcript TEXT NOT NULL,
    status VARCHAR(16) NOT NULL CHECK (status IN ('generating', 'ready', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 1,
    lease_id UUID NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    audio BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, briefing_date)
);
