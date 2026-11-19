-- 000003_synastry_status — per-pair synastry consent + shared daily status.
--
-- Synastry stops being implicit in friendship: a pair may run synastry only
-- after one side requests it AND the other accepts (a request from each side
-- also counts as acceptance). One row per unordered pair (user_lo < user_hi).
-- shared_status is a small self-published JSONB blob (today's calendar score
-- the user chose to share) — same privacy rule as dashboard_prefs: computed
-- results only, never birth rows. IF NOT EXISTS throughout (shared DB).

CREATE TABLE IF NOT EXISTS synastry_consents (
  id VARCHAR(64) PRIMARY KEY,
  user_lo VARCHAR(64) NOT NULL REFERENCES users(id),
  user_hi VARCHAR(64) NOT NULL REFERENCES users(id),
  requested_by VARCHAR(64) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  responded_at TIMESTAMPTZ,
  CONSTRAINT synastry_pair_order CHECK (user_lo < user_hi),
  CONSTRAINT synastry_one_pair UNIQUE (user_lo, user_hi)
);

CREATE INDEX IF NOT EXISTS idx_synastry_lo ON synastry_consents (user_lo, status);
CREATE INDEX IF NOT EXISTS idx_synastry_hi ON synastry_consents (user_hi, status);

ALTER TABLE users ADD COLUMN IF NOT EXISTS shared_status JSONB;
