CREATE TABLE IF NOT EXISTS audio_briefings (
    user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    briefing_date DATE NOT NULL,
    cache_key VARCHAR(64) NOT NULL,
    transcript TEXT NOT NULL,
    status VARCHAR(16) NOT NULL CHECK (status IN ('generating', 'ready', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 1,
    lease_id UUID NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    audio BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, briefing_date)
);
CREATE INDEX IF NOT EXISTS audio_briefings_date_idx ON audio_briefings(briefing_date);
