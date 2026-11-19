-- 000007_seen — per-user "last looked at" marks, so the app can show unread
-- counts for friend requests, synastry requests and wall comments.
--
-- One row per user, created on first mark. Times are UTC; a NULL mark means
-- "never looked", which counts everything as unread. IF NOT EXISTS: shared DB.

CREATE TABLE IF NOT EXISTS social_seen (
  user_id VARCHAR(64) PRIMARY KEY REFERENCES users(id),
  friends_seen_at TIMESTAMPTZ,
  synastry_seen_at TIMESTAMPTZ,
  wall_seen_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
