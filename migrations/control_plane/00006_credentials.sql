-- +goose Up
CREATE TABLE control_plane.credentials (
  id                   uuid PRIMARY KEY DEFAULT uuidv7(),
  account_id           uuid NOT NULL REFERENCES control_plane.accounts (id) ON DELETE CASCADE,
  type                 text NOT NULL CHECK (type IN ('smpp_bind', 'api_key')),
  system_id            text UNIQUE CONSTRAINT credentials_system_id_check
                         CHECK ((type = 'smpp_bind') = (system_id IS NOT NULL) AND system_id ~ '^[\x21-\x7E]{1,15}$'),
  status               text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'revoked')),
  secret_hash          text,
  previous_secret_hash text,
  grace_expires_at     timestamptz,
  last_used_at         timestamptz,
  rotated_at           timestamptz,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id, type),
  CONSTRAINT credentials_revoked_check CHECK ((status = 'revoked') = (secret_hash IS NULL)),
  CONSTRAINT credentials_grace_check CHECK ((previous_secret_hash IS NULL) = (grace_expires_at IS NULL))
);

-- +goose StatementBegin
CREATE FUNCTION control_plane.account_is_closed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (SELECT status FROM control_plane.accounts WHERE id = NEW.account_id FOR SHARE) = 'closed' THEN
    RAISE EXCEPTION 'account % is closed', NEW.account_id
      USING ERRCODE = 'check_violation', CONSTRAINT = 'account_is_closed';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER account_is_closed BEFORE INSERT ON control_plane.credentials
  FOR EACH ROW EXECUTE FUNCTION control_plane.account_is_closed();
CREATE TRIGGER account_is_closed_revival BEFORE UPDATE ON control_plane.credentials
  FOR EACH ROW WHEN (NEW.secret_hash IS DISTINCT FROM OLD.secret_hash AND NEW.secret_hash IS NOT NULL
                     OR NEW.status = 'active' AND OLD.status <> 'active')
  EXECUTE FUNCTION control_plane.account_is_closed();
CREATE TRIGGER touch_updated_at BEFORE UPDATE ON control_plane.credentials
  FOR EACH ROW EXECUTE FUNCTION control_plane.touch_updated_at();
CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.credentials
  FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture();

-- +goose Down
DROP TABLE control_plane.credentials;
DROP FUNCTION control_plane.account_is_closed();
