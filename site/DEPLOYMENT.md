# Deployment Runbook: agy-swap-site

## Prerequisites
- A pinned immutable Docker image digest (e.g., `ghcr.io/aklkbqx/agy-swap-site@sha256:...`)
- `.env.production` configured based on `.env.production.example`.

## 1. Pre-Deployment: Candidate Loopback Test
Before deploying, optionally run the image locally to verify the configuration and assets.
```bash
docker run --rm -d --name agy-swap-site-test -p 127.0.0.1:8080:80 <YOUR_OPS_IMAGE_DIGEST>
curl -I http://localhost:8080/healthz # Should return 200 OK
docker stop agy-swap-site-test
```

## 2. Configuration Validation
Validate the `docker-compose.production.yml` with the production environment variables:
```bash
docker compose -f docker-compose.production.yml --env-file .env.production config
```

## 3. Deployment
Deploy using Compose's wait feature to ensure the healthcheck passes before completing:
```bash
docker compose -f docker-compose.production.yml --env-file .env.production up -d --wait
```

## 4. Post-Deployment Checks
Verify the public origin is serving correctly:
```bash
curl -I https://agy-swap.aklkbqx.com/healthz # Should return 200 OK
curl -I https://agy-swap.aklkbqx.com/ # Should return 200 OK and index.html headers
```

## 5. Rollback
If the deployment fails or checks do not pass, revert to the previous known good image digest in `.env.production` and re-run step 3:
```bash
# After updating .env.production with the old OPS_IMAGE digest
docker compose -f docker-compose.production.yml --env-file .env.production up -d --wait
```
*Note: If this is the first deploy, a rollback consists of bringing the service down, which will return the domain to a baseline 404.*
```bash
docker compose -f docker-compose.production.yml down
```
