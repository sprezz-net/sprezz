-- +goose Up
-- +goose StatementBegin
ALTER TABLE media_attachments ADD COLUMN width INT;
ALTER TABLE media_attachments ADD COLUMN height INT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE media_attachments DROP COLUMN width;
ALTER TABLE media_attachments DROP COLUMN height;
-- +goose StatementEnd
