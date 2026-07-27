# Proxy setup

The proxy is a Go service that hosts the multi-rover NATS hub, WorkOS-authenticated
fleet view, and rover-enrollment flow. It expects three pieces of external state
before `make proxy` will start: Postgres, an operator NKey, and WorkOS AuthKit
credentials. Steps below are one-time per deployment (local or Railway).

## 1. Local Postgres

The proxy talks to Postgres for users, rovers, and per-rover role bindings.

```
docker compose up -d postgres
```

The container exposes 5432 on localhost with `waypoint / dev` credentials and a
`waypoint` database. Data persists in the `waypoint-pg-data` volume across
`docker compose down`.

Migrations run automatically when the proxy starts (embedded via `goose`).

## 2. Operator NKey

The proxy's NATS hub runs in **operator mode**: it trusts account JWTs signed
by a single root key (the operator). One operator key is generated once per
deployment and never rotated casually.

Generate it:

```
make proxy-init-operator
```

Two lines are printed:

```
OPERATOR_NKEY_SEED=<base64>
OPERATOR_PUBLIC_KEY=O...
```

- `OPERATOR_NKEY_SEED` is the secret. Drop it into `proxy/.env` and into the
  Railway env vars. Never commit it.
- `OPERATOR_PUBLIC_KEY` is safe to commit and ends up baked into every rover's
  `identity.toml` so the rover can verify magic-link tokens.

The proxy refuses to start without `OPERATOR_NKEY_SEED`.

## 3. WorkOS AuthKit

Users sign in to the proxy via WorkOS AuthKit. Passwords are never handled
directly; AuthKit returns a session that the proxy upserts into the `users` table.

1. Create a WorkOS account at https://dashboard.workos.com.
2. Create a project named **Waypoint**.
3. **Authentication → AuthKit**: enable AuthKit.
4. **Redirects**: add `http://localhost:8081/auth/callback`.
   After Railway is live, also add `https://<railway-url>/auth/callback`.
5. Copy:
   - **API key** (`sk_...`) → `WORKOS_API_KEY`
   - **Client ID** (`client_...`) → `WORKOS_CLIENT_ID`
6. Generate a cookie-encryption key (32 bytes, base64):
   ```
   openssl rand -base64 32
   ```
   Set it as `WORKOS_COOKIE_PASSWORD`.

## 4. Fill `proxy/.env`

```
cp proxy/.env.example proxy/.env
```

Fill in every variable. The proxy validates required env vars at startup and
will print `env: X is required` and exit if any are missing.

## 5. Start the proxy

```
make proxy
```

You should see `waypoint-proxy listening on :8081`. Visit
http://localhost:8081; the first sign-in becomes the admin user.

## First-user-becomes-admin

The very first row inserted into `users` (via the first WorkOS sign-in) is
flagged `is_admin = true`. Every subsequent user is non-admin; the admin can
grant per-rover `monitor` / `control` / `admin` roles from the Admin view.

There is no admin-promotion UI today; if you lose admin access, the recovery
path is direct SQL: `UPDATE users SET is_admin = TRUE WHERE email = ...`.

## Railway deployment

The proxy ships as a single Docker image built by `proxy/Dockerfile` (multi-stage:
it builds the dashboard with pnpm, then embeds `dist/` into the Go binary).

1. Create a Railway project named **Waypoint Proxy**.
2. Connect this GitHub repo. Set the **root directory** to the repo root and
   the **Dockerfile path** to `proxy/Dockerfile`.
3. Add the **Postgres** add-on. Copy the connection string Railway provides.
4. Add these environment variables in the Railway service settings:
   - `DATABASE_URL`: Railway's Postgres URL.
   - `OPERATOR_NKEY_SEED`, `OPERATOR_PUBLIC_KEY`: from `make proxy-init-operator`.
     **Generate once, use the same seed for the life of the deployment.**
   - `WORKOS_API_KEY`, `WORKOS_CLIENT_ID`: from your WorkOS dashboard.
   - `WORKOS_REDIRECT_URI`: `https://<railway-url>/auth/callback`. Once
     Railway prints the deployment URL, also add this URL to the **Redirects**
     list in the WorkOS dashboard.
   - `WORKOS_COOKIE_PASSWORD`: `openssl rand -base64 32`.
   - `PUBLIC_ORIGIN`: `https://<railway-url>`.
5. Trigger the first deploy. Once `/healthz` returns `ok`, the proxy is live.
6. Optional: Add a Railway API token as the `RAILWAY_TOKEN` repo secret on
   GitHub. Pushing a tag matching `proxy-v*` then triggers
   `.github/workflows/proxy-deploy.yml` for an auto-deploy.

`make fleet-smoke` prints the end-to-end checklist for first-rover enrollment.
