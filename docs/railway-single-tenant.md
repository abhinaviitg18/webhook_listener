# Railway Single-Tenant Deployment

This mode runs AgentHook as a private operator console for one partner-owned Railway service. It is enabled only when `APP_DEPLOYMENT_MODE=single_tenant`, so the existing AWS/ScaleKit deployment remains unchanged.

## Required Railway Variables

```env
APP_DEPLOYMENT_MODE=single_tenant
APP_PLAN=enterprise
PUBLIC_BASE_URL=https://agenthook.partner-domain.com
SINGLE_TENANT_OWNER_EMAIL=ops@partner-domain.com
SINGLE_TENANT_OWNER_ALIAS=partner
SINGLE_TENANT_SETUP_TOKEN_SHA256=<sha256-of-setup-token>
ALLOW_PUBLIC_REGISTRATION=false
COMMERCE_MYSQL_DSN=<mysql-or-tidb-dsn>
```

`PUBLIC_BASE_URL` should be the partner custom domain for production. The Railway-generated domain is fine for temporary smoke tests.

## First Login

1. Open `PUBLIC_BASE_URL`.
2. Enter the setup token whose SHA-256 hash is stored in `SINGLE_TENANT_SETUP_TOKEN_SHA256`.
3. AgentHook creates or reuses `SINGLE_TENANT_OWNER_EMAIL`, then sets the existing `htc_token` session cookie.
4. Create an `AGENTHOOK_TOKEN` from the home console for CLIs, scripts, and agents.

## Rotating The Setup Token

Generate a new token, hash it, update `SINGLE_TENANT_SETUP_TOKEN_SHA256`, and redeploy:

```bash
printf '%s' 'new-setup-token' | shasum -a 256
```

Existing browser sessions and API tokens are not revoked by setup-token rotation. Revoke API tokens from the UI if needed.

## Deploy

The current deployment path uses source upload with the repo `Dockerfile`:

```bash
RAILWAY_TOKEN=... \
RAILWAY_PROJECT=... \
RAILWAY_ENVIRONMENT=production \
RAILWAY_SERVICE=agenthook \
bash scripts/railway/deploy.sh
```

The script syncs supported environment variables before running `railway up`.
