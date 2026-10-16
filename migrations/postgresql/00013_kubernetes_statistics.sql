-- +goose Up
ALTER TABLE kubernetes_clusters ADD COLUMN statistics_observed_at timestamptz;

-- +goose Down
ALTER TABLE kubernetes_clusters DROP COLUMN statistics_observed_at;
