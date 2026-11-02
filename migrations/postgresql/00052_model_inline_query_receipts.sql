-- +goose Up
ALTER TABLE model_calls ADD COLUMN inline_query_receipts jsonb NOT NULL DEFAULT '[]'::jsonb
 CHECK (jsonb_typeof(inline_query_receipts)='array' AND jsonb_array_length(inline_query_receipts)<=64);
COMMENT ON COLUMN model_calls.inline_query_receipts IS 'Private framework receipts for complete native originals actually included in this provider request; IDs/hash/size only, no contents or credentials';

-- +goose Down
ALTER TABLE model_calls DROP COLUMN inline_query_receipts;
