-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION control_plane.customer_is_closed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (SELECT status FROM control_plane.customers WHERE id = NEW.customer_id FOR SHARE) = 'closed' THEN
    RAISE EXCEPTION 'customer % is closed', NEW.customer_id
      USING ERRCODE = 'check_violation', CONSTRAINT = 'customer_is_closed';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER customer_is_closed BEFORE INSERT ON control_plane.accounts
  FOR EACH ROW EXECUTE FUNCTION control_plane.customer_is_closed();

-- +goose Down
DROP TRIGGER customer_is_closed ON control_plane.accounts;
DROP FUNCTION control_plane.customer_is_closed();
