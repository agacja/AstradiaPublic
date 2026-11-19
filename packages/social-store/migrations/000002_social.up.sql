-- 000002_social — friendships + dashboard prefs.
--
-- House rules: no secret columns here, so no BYTEA — links and
-- widget preferences only. Birth data stays in users and NEVER crosses the API:
-- friends receive computed results (scores, aspect names, interpretation text),
-- never raw birth rows. IF NOT EXISTS throughout: the TS server may share this
-- database and this must be a no-op on an already-migrated DB.

CREATE TABLE IF NOT EXISTS friendships (
  id VARCHAR(64) PRIMARY KEY,
  requester_id VARCHAR(64) NOT NULL REFERENCES users(id),
  addressee_id VARCHAR(64) NOT NULL REFERENCES users(id),
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  responded_at TIMESTAMPTZ,
  CONSTRAINT friendships_no_self CHECK (requester_id <> addressee_id),
  CONSTRAINT friendships_one_edge UNIQUE (requester_id, addressee_id)
);

CREATE INDEX IF NOT EXISTS idx_friendships_requester ON friendships (requester_id, status);
CREATE INDEX IF NOT EXISTS idx_friendships_addressee ON friendships (addressee_id, status);

ALTER TABLE users ADD COLUMN IF NOT EXISTS dashboard_prefs JSONB;
