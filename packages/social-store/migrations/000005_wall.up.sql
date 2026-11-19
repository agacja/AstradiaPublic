-- 000005_wall — profile comment wall (the MySpace feature).
--
-- Friends of a profile's owner may write on it; the owner (or the author)
-- may delete. Bodies are plain text, length-capped at the API. IF NOT EXISTS:
-- shared DB.

CREATE TABLE IF NOT EXISTS profile_comments (
  id VARCHAR(64) PRIMARY KEY,
  profile_id VARCHAR(64) NOT NULL REFERENCES users(id),
  author_id  VARCHAR(64) NOT NULL REFERENCES users(id),
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wall_profile ON profile_comments (profile_id, created_at DESC);
