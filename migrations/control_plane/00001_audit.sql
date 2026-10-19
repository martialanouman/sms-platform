-- +goose Up
CREATE SCHEMA control_plane;

CREATE TABLE control_plane.audit_log (
  id           uuid NOT NULL DEFAULT uuidv7(),
  operator     text NOT NULL,
  operation_id text NOT NULL,
  target       text NOT NULL,
  request_id   text,
  status       smallint NOT NULL CHECK (status BETWEEN 100 AND 599),
  outcome      text,
  at           timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (at, id)
);

CREATE TABLE control_plane.audit_changes (
  id         uuid NOT NULL DEFAULT uuidv7(),
  request_id text,
  actor      text NOT NULL,
  table_name text NOT NULL,
  pk         jsonb NOT NULL,
  op         text NOT NULL CHECK (op IN ('insert', 'update', 'delete')),
  before     jsonb,
  after      jsonb,
  at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (at, id)
);

-- ADR-0018: the floor is in hours, as absolute as the purge's own cutoff.
-- +goose StatementBegin
CREATE FUNCTION control_plane.audit_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' AND current_setting('audit_log.purge', true) = 'on'
     AND OLD.at < now() - interval '8760 hours' THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION '%.% is append-only: % refused', TG_TABLE_SCHEMA, TG_TABLE_NAME, TG_OP
    USING ERRCODE = 'insufficient_privilege';
END $$;
-- +goose StatementEnd

CREATE TRIGGER audit_append_only BEFORE UPDATE OR DELETE ON control_plane.audit_log
  FOR EACH ROW EXECUTE FUNCTION control_plane.audit_append_only();
CREATE TRIGGER audit_no_truncate BEFORE TRUNCATE ON control_plane.audit_log
  FOR EACH STATEMENT EXECUTE FUNCTION control_plane.audit_append_only();
CREATE TRIGGER audit_append_only BEFORE UPDATE OR DELETE ON control_plane.audit_changes
  FOR EACH ROW EXECUTE FUNCTION control_plane.audit_append_only();
CREATE TRIGGER audit_no_truncate BEFORE TRUNCATE ON control_plane.audit_changes
  FOR EACH STATEMENT EXECUTE FUNCTION control_plane.audit_append_only();

-- +goose Down
DROP SCHEMA control_plane CASCADE;
