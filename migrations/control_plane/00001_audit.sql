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

-- I13. SECURITY DEFINER: the journal is written as its owner, so no service holds a right on it.
-- +goose StatementBegin
CREATE FUNCTION control_plane.audit_capture() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
  actor text := current_setting('app.actor', true);
  sealed text[];
  row_before jsonb;
  row_after jsonb;
  pk jsonb;
BEGIN
  IF actor IS NULL OR actor = '' THEN
    RAISE EXCEPTION 'I13: write to %.% without app.actor', TG_TABLE_SCHEMA, TG_TABLE_NAME
      USING ERRCODE = 'invalid_parameter_value';
  END IF;
  sealed := ARRAY(SELECT attname FROM pg_attribute
    WHERE attrelid = TG_RELID AND attnum > 0 AND NOT attisdropped
      AND (attname ~ '(_sealed|_hash|_kms_key_ref)$' OR attname = 'wrapped_key'));
  IF TG_OP <> 'INSERT' THEN row_before := to_jsonb(OLD) - sealed; END IF;
  IF TG_OP <> 'DELETE' THEN row_after := to_jsonb(NEW) - sealed; END IF;
  SELECT jsonb_object_agg(a.attname, coalesce(row_after, row_before) -> a.attname::text) INTO pk
    FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
    WHERE i.indrelid = TG_RELID AND i.indisprimary;
  INSERT INTO control_plane.audit_changes (request_id, actor, table_name, pk, op, before, after)
    VALUES (nullif(current_setting('app.request_id', true), ''), actor, TG_TABLE_NAME, pk,
            lower(TG_OP), row_before, row_after);
  PERFORM pg_notify('config', TG_TABLE_NAME || ':' || pk::text);
  RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP SCHEMA control_plane CASCADE;
