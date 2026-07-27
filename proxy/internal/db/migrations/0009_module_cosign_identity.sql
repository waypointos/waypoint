-- +goose Up
-- Switch module trust from a stored ed25519 key to a pinned cosign-keyless
-- workflow identity (the repo's release-workflow OIDC SAN).
ALTER TABLE module_registry DROP COLUMN signing_key_pub;
ALTER TABLE module_registry DROP COLUMN signing_key_fpr;
ALTER TABLE module_registry ADD COLUMN expected_identity_san TEXT NOT NULL DEFAULT '';
ALTER TABLE module_registry ALTER COLUMN expected_identity_san DROP DEFAULT;

-- +goose Down
ALTER TABLE module_registry DROP COLUMN expected_identity_san;
ALTER TABLE module_registry ADD COLUMN signing_key_pub TEXT NOT NULL DEFAULT '';
ALTER TABLE module_registry ADD COLUMN signing_key_fpr TEXT NOT NULL DEFAULT '';
