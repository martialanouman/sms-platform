-- +goose Up
ALTER TABLE dashboard.sessions ADD COLUMN elevated_at timestamptz;

CREATE TABLE dashboard.totp_factors (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  operator_id  uuid NOT NULL REFERENCES dashboard.operators (id),
  sealed       bytea NOT NULL,
  last_step    bigint NOT NULL DEFAULT 0,
  confirmed_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX totp_factors_confirmed ON dashboard.totp_factors (operator_id) WHERE confirmed_at IS NOT NULL;
CREATE UNIQUE INDEX totp_factors_pending ON dashboard.totp_factors (operator_id) WHERE confirmed_at IS NULL;

CREATE TABLE dashboard.recovery_codes (
  digest    bytea PRIMARY KEY,
  factor_id uuid NOT NULL REFERENCES dashboard.totp_factors (id) ON DELETE CASCADE,
  used_at   timestamptz
);
CREATE INDEX recovery_codes_factor ON dashboard.recovery_codes (factor_id);

-- +goose Down
DROP TABLE dashboard.recovery_codes;
DROP TABLE dashboard.totp_factors;
ALTER TABLE dashboard.sessions DROP COLUMN elevated_at;
