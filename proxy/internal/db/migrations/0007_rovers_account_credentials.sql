-- +goose Up
--
-- Persist each rover's NATS account credentials (the account JWT + the
-- account NKey seed) on the rovers row.
--
-- Both fields previously lived only in process memory: the hub's
-- MemAccResolver held the JWT, and the operator's keypair cache held
-- the seed. A proxy restart cleared both, leaving previously-enrolled
-- rovers unable to authenticate until re-enrollment.
--
-- Startup now lists rovers, restores each account keypair into the
-- operator cache (so MintRoverUserJWT can still sign user JWTs under
-- the same account), and re-registers each account JWT with the hub's
-- resolver (so previously-issued user JWTs continue to validate).
--
-- Both columns are nullable to keep this migration non-destructive.
-- Rows enrolled before the migration ran will have NULL credentials;
-- startup logs and skips them, and they need to be re-enrolled to come
-- back online. Rows enrolled after the migration always populate both.
ALTER TABLE rovers
    ADD COLUMN account_jwt  TEXT,
    ADD COLUMN account_seed TEXT;

-- +goose Down
ALTER TABLE rovers
    DROP COLUMN account_seed,
    DROP COLUMN account_jwt;
