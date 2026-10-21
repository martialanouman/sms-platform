-- +goose Up
CREATE TABLE dashboard.roles (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  name        text NOT NULL UNIQUE CHECK (char_length(name) BETWEEN 1 AND 100),
  description text NOT NULL,
  permissions text[] NOT NULL,
  is_default  boolean NOT NULL DEFAULT false
);

CREATE TABLE dashboard.operator_roles (
  operator_id uuid NOT NULL REFERENCES dashboard.operators (id),
  role_id     uuid NOT NULL REFERENCES dashboard.roles (id),
  PRIMARY KEY (operator_id, role_id)
);
CREATE INDEX operator_roles_role ON dashboard.operator_roles (role_id);

-- +goose Down
DROP TABLE dashboard.operator_roles;
DROP TABLE dashboard.roles;
