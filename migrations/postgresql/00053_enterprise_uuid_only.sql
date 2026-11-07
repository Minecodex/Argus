-- +goose Up
ALTER TABLE enterprises DROP COLUMN code;

-- +goose Down
ALTER TABLE enterprises ADD COLUMN code text;
UPDATE enterprises SET code = id::text;
ALTER TABLE enterprises
    ALTER COLUMN code SET NOT NULL,
    ADD CONSTRAINT enterprises_code_key UNIQUE (code),
    ADD CONSTRAINT enterprises_code_check CHECK (
        char_length(code) BETWEEN 1 AND 63 AND code ~ '^[a-z0-9]+(-[a-z0-9]+)*$'
    );
