-- +goose Up
ALTER TABLE messages RENAME COLUMN body TO content;

-- +goose Down
ALTER TABLE messages RENAME COLUMN content TO body;
