-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION control_plane.actor_operator() RETURNS uuid LANGUAGE sql STABLE AS $$
  SELECT CASE WHEN a ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN a::uuid END
  FROM current_setting('app.actor', true) AS a
$$;
-- +goose StatementEnd

CREATE TABLE control_plane.customer_groups (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  name        text NOT NULL UNIQUE,
  description text,
  status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
  created_by  uuid DEFAULT control_plane.actor_operator(),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE control_plane.customers (
  id                    uuid PRIMARY KEY DEFAULT uuidv7(),
  name                  text NOT NULL,
  status                text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'closed')),
  group_id              uuid REFERENCES control_plane.customer_groups (id) ON DELETE SET NULL,
  max_age_otp           integer CHECK (max_age_otp BETWEEN 1 AND 600),
  max_age_transactional integer CHECK (max_age_transactional BETWEEN 1 AND 86400),
  max_age_marketing     integer CHECK (max_age_marketing BETWEEN 1 AND 259200),
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX customers_group_id ON control_plane.customers (group_id, id);

CREATE TABLE control_plane.accounts (
  id                 uuid PRIMARY KEY DEFAULT uuidv7(),
  customer_id        uuid NOT NULL REFERENCES control_plane.customers (id) ON DELETE RESTRICT,
  name               text NOT NULL,
  status             text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'closed')),
  smpp_enabled       boolean NOT NULL DEFAULT true,
  rest_enabled       boolean NOT NULL DEFAULT true,
  query_sm_enabled   boolean NOT NULL DEFAULT true,
  cancel_sm_enabled  boolean NOT NULL DEFAULT true,
  allowed_bind_types text[] NOT NULL DEFAULT '{transceiver}' CONSTRAINT accounts_allowed_bind_types_check
                       CHECK (cardinality(allowed_bind_types) >= 1
                              AND allowed_bind_types <@ ARRAY['transmitter', 'receiver', 'transceiver']),
  max_sessions       integer NOT NULL DEFAULT 1 CHECK (max_sessions >= 0),
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT accounts_channel_check CHECK (smpp_enabled OR rest_enabled),
  UNIQUE (customer_id, name)
);

-- +goose StatementBegin
CREATE FUNCTION control_plane.closed_is_final() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status = 'closed' AND NEW.status <> 'closed' THEN
    RAISE EXCEPTION '% % is closed', TG_TABLE_NAME, OLD.id
      USING ERRCODE = 'check_violation', CONSTRAINT = 'closed_is_final';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION control_plane.touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := clock_timestamp();
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER closed_is_final BEFORE UPDATE OF status ON control_plane.customers
  FOR EACH ROW EXECUTE FUNCTION control_plane.closed_is_final();
CREATE TRIGGER closed_is_final BEFORE UPDATE OF status ON control_plane.accounts
  FOR EACH ROW EXECUTE FUNCTION control_plane.closed_is_final();

CREATE TRIGGER touch_updated_at BEFORE UPDATE ON control_plane.customer_groups
  FOR EACH ROW EXECUTE FUNCTION control_plane.touch_updated_at();
CREATE TRIGGER touch_updated_at BEFORE UPDATE ON control_plane.customers
  FOR EACH ROW EXECUTE FUNCTION control_plane.touch_updated_at();
CREATE TRIGGER touch_updated_at BEFORE UPDATE ON control_plane.accounts
  FOR EACH ROW EXECUTE FUNCTION control_plane.touch_updated_at();

CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.customer_groups
  FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture();
CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.customers
  FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture();
CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.accounts
  FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture();

-- +goose Down
DROP TABLE control_plane.accounts;
DROP TABLE control_plane.customers;
DROP TABLE control_plane.customer_groups;
DROP FUNCTION control_plane.touch_updated_at();
DROP FUNCTION control_plane.closed_is_final();
DROP FUNCTION control_plane.actor_operator();
