-- +goose Up
CREATE INDEX audit_log_request_id ON control_plane.audit_log (request_id);
CREATE INDEX audit_changes_request_id ON control_plane.audit_changes (request_id, at, id);

-- +goose Down
DROP INDEX control_plane.audit_changes_request_id;
DROP INDEX control_plane.audit_log_request_id;
