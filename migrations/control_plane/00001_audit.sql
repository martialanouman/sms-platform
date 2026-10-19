-- +goose Up
CREATE SCHEMA control_plane;

-- +goose Down
DROP SCHEMA control_plane CASCADE;
