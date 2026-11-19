-- 000006_nick — public display name, so emails never leave the server.
--
-- Every social surface (friends, Top 8, wall, synastry) now shows a nick.
-- Users without one get a stable anonymous handle derived from their id.
-- Case-insensitively unique. IF NOT EXISTS: shared DB.

ALTER TABLE users ADD COLUMN IF NOT EXISTS nick VARCHAR(24);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_nick_lower ON users (LOWER(nick)) WHERE nick IS NOT NULL;
