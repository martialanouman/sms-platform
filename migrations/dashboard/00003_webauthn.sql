-- +goose Up
CREATE TABLE dashboard.webauthn_credentials (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  operator_id   uuid NOT NULL REFERENCES dashboard.operators (id),
  credential_id bytea NOT NULL UNIQUE,
  name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
  sign_count    bigint NOT NULL DEFAULT 0,
  credential    jsonb NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webauthn_credentials_operator ON dashboard.webauthn_credentials (operator_id);

CREATE TABLE dashboard.webauthn_challenges (
  session_id uuid NOT NULL REFERENCES dashboard.sessions (id) ON DELETE CASCADE,
  purpose    text NOT NULL CHECK (purpose IN ('registration', 'assertion')),
  ceremony   jsonb NOT NULL,
  PRIMARY KEY (session_id, purpose)
);

-- +goose Down
DROP TABLE dashboard.webauthn_challenges;
DROP TABLE dashboard.webauthn_credentials;
