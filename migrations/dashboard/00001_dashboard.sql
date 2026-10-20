-- +goose Up
REVOKE ALL ON DATABASE dashboard FROM PUBLIC;

CREATE SCHEMA dashboard;

CREATE TABLE dashboard.operators (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  email         text NOT NULL,
  display_name  text NOT NULL,
  password_hash text,
  status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX operators_email ON dashboard.operators (lower(email));

CREATE TABLE dashboard.access_links (
  operator_id     uuid PRIMARY KEY REFERENCES dashboard.operators (id),
  kind            text NOT NULL CHECK (kind IN ('activation', 'reset')),
  token_hash      bytea UNIQUE,
  expires_at      timestamptz,
  sent_at         timestamptz,
  attempts        int NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE dashboard.sessions (
  id                   uuid PRIMARY KEY DEFAULT uuidv7(),
  operator_id          uuid NOT NULL REFERENCES dashboard.operators (id),
  token_hash           bytea NOT NULL UNIQUE,
  created_at           timestamptz NOT NULL DEFAULT now(),
  expires_at           timestamptz NOT NULL,
  last_seen_at         timestamptz NOT NULL DEFAULT now(),
  challenge_hash       bytea UNIQUE,
  challenge_expires_at timestamptz
);

CREATE TABLE dashboard.login_attempt_counters (
  subject         bytea PRIMARY KEY,
  failures        int NOT NULL,
  last_failure_at timestamptz NOT NULL
);

CREATE TABLE dashboard.access_log (
  id      uuid NOT NULL DEFAULT uuidv7(),
  at      timestamptz NOT NULL DEFAULT now(),
  actor   uuid,
  event   text NOT NULL CHECK (event IN ('login', 'logout', 'mfa', 'access_link_requested', 'access_link_used',
    'factor_enrolled', 'factor_removed', 'operator_created', 'operator_disabled', 'operator_enabled', 'roles_changed',
    'role_created', 'role_updated', 'role_deleted', 'session_closed', 'personal_token_issued', 'session_refused')),
  target  text,
  outcome text NOT NULL CHECK (outcome IN ('success', 'failure')),
  PRIMARY KEY (at, id)
);

-- +goose StatementBegin
CREATE FUNCTION dashboard.access_log_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' AND current_setting('audit_log.purge', true) = 'on'
     AND OLD.at < now() - interval '8760 hours' THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION '%.% is append-only: % refused', TG_TABLE_SCHEMA, TG_TABLE_NAME, TG_OP
    USING ERRCODE = 'insufficient_privilege';
END $$;
-- +goose StatementEnd

CREATE TRIGGER access_log_append_only BEFORE UPDATE OR DELETE ON dashboard.access_log
  FOR EACH ROW EXECUTE FUNCTION dashboard.access_log_append_only();
CREATE TRIGGER access_log_no_truncate BEFORE TRUNCATE ON dashboard.access_log
  FOR EACH STATEMENT EXECUTE FUNCTION dashboard.access_log_append_only();

-- +goose Down
DROP SCHEMA dashboard CASCADE;
GRANT CONNECT, TEMPORARY ON DATABASE dashboard TO PUBLIC;
