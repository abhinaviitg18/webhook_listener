# Railway Single-Tenant Deployment

This mode runs AgentHook as a private operator console for one partner-owned Railway service. It is enabled only when `APP_DEPLOYMENT_MODE=single_tenant`, so the existing AWS/ScaleKit deployment remains unchanged.

## Required Railway Variables

The production template should include a Railway MySQL service named `MySQL` and set:

```env
COMMERCE_MYSQL_DSN=root:${{ MySQL.MYSQL_ROOT_PASSWORD }}@tcp(${{ MySQL.RAILWAY_PRIVATE_DOMAIN }}:3306)/railway?parseTime=true
```

AgentHook creates the `railway` database when needed, then runs embedded schema migrations at startup before serving traffic. A fresh Railway MySQL database does not require any manual SQL setup.
Railway's `mysql://...` URL is also accepted directly and normalized internally for the Go MySQL driver.

The only value the template installer should need to enter is:

```env
SINGLE_TENANT_OWNER_EMAIL=ops@partner-domain.com
```

When `SINGLE_TENANT_OWNER_EMAIL` is set, AgentHook infers single-tenant mode, defaults the plan to Enterprise, disables public registration, uses Railway MySQL, and derives the public base URL from the current request host unless `PUBLIC_BASE_URL` is explicitly set.

`PUBLIC_BASE_URL` may still be set after deployment for a partner custom domain. The Railway-generated domain is fine for temporary smoke tests.

## Database Options

The default partner template should create Railway MySQL automatically. Advanced users can bring an existing MySQL-compatible database by overriding `COMMERCE_MYSQL_DSN` with their own DSN after deployment.

`USE_IN_MEMORY_STORE=true` is only for local development and temporary demos. It avoids database cost, but data is lost on restart or redeploy.

## First Login

1. Open the `agenthook` service logs in Railway after the first deploy.
2. Copy the one-time `claim_code` value printed by AgentHook.
3. Open the service URL and enter the claim code.
4. AgentHook creates or reuses `SINGLE_TENANT_OWNER_EMAIL`, consumes the claim, then sets the existing `htc_token` session cookie.
5. Create an `AGENTHOOK_TOKEN` from the home console for CLIs, scripts, and agents.

## Legacy Setup Token

`SINGLE_TENANT_SETUP_TOKEN_SHA256` is still supported for older installs. If it is set, the app uses setup-token login instead of the one-time claim flow.

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
