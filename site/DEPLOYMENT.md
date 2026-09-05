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

## 6. Indexing (Google Search)

Code deploy does not put the URL in Google’s index. Do these operator steps on the live host; do not commit verification tokens.

### Search Console

1. Add a URL-prefix property for `https://agy-swap.aklkbqx.com/`.
2. Verify with a Cloudflare DNS TXT record on `agy-swap.aklkbqx.com`. Do not put that token in this repo.
3. Submit `https://agy-swap.aklkbqx.com/sitemap.xml`.
4. URL Inspection → `https://agy-swap.aklkbqx.com/` → Request indexing.
5. After this image is live, inspect `https://agy-swap.aklkbqx.com/install.html` the same way.

Do not ping `https://www.google.com/ping?sitemap=...`. That endpoint is retired.

### Cloudflare apex redirect

`https://aklkbqx.com/` currently 404s, so Google has no path from the registered domain to this site. MX is unaffected.

Create a Cloudflare Redirect Rule (edge 301, not Traefik — Traefik only routes `Host(agy-swap.aklkbqx.com)` and would need an origin cert for the apex):

- If hostname is `aklkbqx.com` or `www.aklkbqx.com`
- Then dynamic redirect (status 301) to `https://agy-swap.aklkbqx.com/${uri.path}` (preserve query if the UI offers it)

If the apex must remain a personal homepage later, replace this 301 with a one-page index that links here. Do not leave the 404.

### Cloudflare crawler hygiene

- Turn **off** Email Address Obfuscation for this hostname.
- After any WAF / Bot Fight change, confirm Googlebot still gets HTTP 200 (not a challenge page).

### Post-deploy checks

```bash
curl -sI https://agy-swap.aklkbqx.com/
curl -sI https://agy-swap.aklkbqx.com/robots.txt
curl -s https://agy-swap.aklkbqx.com/sitemap.xml
curl -sI https://agy-swap.aklkbqx.com/install.html
curl -sI https://aklkbqx.com/   # expect 301 to agy-swap after the Cloudflare rule
curl -sI -A "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)" https://agy-swap.aklkbqx.com/
```

The Googlebot HTML body must not contain `noindex` or `email-protection`.

Success: Search Console reports “URL is on Google”, and `site:agy-swap.aklkbqx.com` returns the homepage (often 1–14 days after Request indexing). Brand queries such as `agy-swap antigravity` are the realistic ranking target; generic `agy-swap` collides with crypto DEX results.
