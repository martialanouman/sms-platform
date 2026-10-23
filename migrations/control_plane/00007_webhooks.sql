-- +goose Up
CREATE TABLE control_plane.webhooks (
  id                 uuid PRIMARY KEY DEFAULT uuidv7(),
  account_id         uuid NOT NULL REFERENCES control_plane.accounts (id) ON DELETE CASCADE,
  event_type         text NOT NULL CHECK (event_type IN ('mo', 'dlr')),
  url                text NOT NULL CONSTRAINT webhooks_url_check
                       CHECK (url ~ '^https://[^/?#@]+([/?#].*)?$' AND length(url) <= 2048),
  status             text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  secret_sealed      bytea NOT NULL,
  secret_kms_key_ref text NOT NULL CHECK (secret_kms_key_ref <> ''),
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id, event_type)
);

CREATE TRIGGER account_is_closed BEFORE INSERT ON control_plane.webhooks
  FOR EACH ROW EXECUTE FUNCTION control_plane.account_is_closed();
CREATE TRIGGER account_is_closed_revival BEFORE UPDATE ON control_plane.webhooks
  FOR EACH ROW WHEN (NEW.secret_sealed IS DISTINCT FROM OLD.secret_sealed
                     OR NEW.status = 'active' AND OLD.status <> 'active')
  EXECUTE FUNCTION control_plane.account_is_closed();
CREATE TRIGGER touch_updated_at BEFORE UPDATE ON control_plane.webhooks
  FOR EACH ROW EXECUTE FUNCTION control_plane.touch_updated_at();
CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.webhooks
  FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture();

-- +goose Down
DROP TABLE control_plane.webhooks;
