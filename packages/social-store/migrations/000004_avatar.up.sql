-- 000004_avatar — small profile picture, stored inline.
--
-- BYTEA on users: avatars are client-downscaled thumbnails (~10–30KB), served
-- back inline as data URLs inside authenticated social responses — no public
-- image endpoint, so the same friendship gates that protect scores protect
-- faces. IF NOT EXISTS: shared DB.

ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar BYTEA;
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_mime VARCHAR(32);
