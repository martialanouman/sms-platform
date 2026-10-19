-- +goose Up
-- A key change notifies the old key too: a snapshot would otherwise keep the old row until its resync.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION control_plane.audit_capture() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS $$
DECLARE
  actor text := current_setting('app.actor', true);
  sealed text[];
  row_before jsonb;
  row_after jsonb;
  pk jsonb;
  old_pk jsonb;
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
  IF TG_OP = 'UPDATE' THEN
    SELECT jsonb_object_agg(a.attname, row_before -> a.attname::text) INTO old_pk
      FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
      WHERE i.indrelid = TG_RELID AND i.indisprimary;
    IF old_pk IS DISTINCT FROM pk THEN
      PERFORM pg_notify('config', TG_TABLE_NAME || ':' || old_pk::text);
    END IF;
  END IF;
  PERFORM pg_notify('config', TG_TABLE_NAME || ':' || pk::text);
  RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION control_plane.audit_capture() RETURNS trigger
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
