-- +goose Up
ALTER TABLE messages RENAME COLUMN content TO body;

-- +goose Down
ALTER TABLE messages RENAME COLUMN body TO content;
