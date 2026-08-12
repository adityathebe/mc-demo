-- +goose Up
CREATE TABLE messages (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO messages (body) VALUES
    ('Mission Control sees infrastructure, source code, and database state together.'),
    ('This healthy baseline will later receive an intentionally breaking migration.');

-- +goose Down
DROP TABLE messages;
