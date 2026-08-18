# QR Social Profile Platform

One stable QR code opens a fast, branded profile page with all social, contact
and business links. The QR encodes the canonical profile URL
(`BASE_URL/{slug}`), so links and branding can change without reprinting
anything.

Seeded profile: **Travel Door** at `/traveldoor`.

## Run with Docker

Nothing to configure — the compose file has working defaults and the container
seeds itself:

```bash
docker compose -f docker-compose.local.yml up --build
```

The default `docker-compose.yml` is the Dokploy/production stack, so local runs
name the local file explicitly.

Then open <http://localhost:8090/traveldoor> (public page) and
<http://localhost:8090/admin> (sign in with `office@traveldoor.ge` /
`change-me-now-please`).

The host port is **8090**, not 8080, because 8080 is commonly already in use.
Inside the container the app always listens on 8080.

Useful commands:

```bash
docker compose -f docker-compose.local.yml logs -f
```

```bash
docker compose -f docker-compose.local.yml down
```

```bash
docker compose -f docker-compose.local.yml down -v
```

`down` keeps the database and uploads in named volumes; `down -v` deletes them
so the next start seeds from scratch.

### Overrides

All optional, all `QR_`-prefixed so they cannot collide with the app's own
`.env` (which Compose reads for substitution):

| Variable | Default | Notes |
| -------- | ------- | ----- |
| `QR_HOST_PORT` | `8090` | host port mapped to the container's 8080 |
| `QR_BASE_URL` | `http://localhost:8090` | must match how the browser reaches the app |
| `QR_APP_ENV` | `development` | `production` additionally requires an `https://` `QR_BASE_URL` and a real secret |
| `QR_SESSION_SECRET` | insecure local value | `openssl rand -base64 32` |
| `QR_ADMIN_EMAIL` / `QR_ADMIN_PASSWORD` | `office@traveldoor.ge` / `change-me-now-please` | only used when no user exists yet |
| `QR_SEED_DEFAULT` | `true` | seed only when the profile is missing (`force` to re-import) |

```bash
QR_HOST_PORT=9000 QR_BASE_URL=http://localhost:9000 docker compose -f docker-compose.local.yml up --build
```

### Scanning the QR from a phone

The QR encodes `BASE_URL/{slug}`, so a code generated with
`http://localhost:8090` will not resolve on a phone. Start with your machine's
LAN address instead:

```bash
QR_BASE_URL=http://192.168.1.50:8090 docker compose -f docker-compose.local.yml up -d
```

Then regenerate the QR from the admin page and scan it from the same network.

## Deploy to a Dokploy VPS

Target: <https://qrcode.binomargroup.com>

### 1. DNS first

Point an `A` record for `qrcode` at your VPS IP before deploying, or the
Let's Encrypt challenge fails and Dokploy shows a certificate error.

| Type | Name    | Value           | Proxy |
| ---- | ------- | --------------- | ----- |
| A    | qrcode  | your VPS IP     | off / DNS-only initially |

### 2. Create the application

In Dokploy: **Create Application** → source **Git** (this repository) or
**Docker file**, build type **Dockerfile**, path `./Dockerfile`. Nothing else
about the build needs configuring — templates, CSS, migrations, the seed data
and the logo are all embedded in the binary.

### 3. Environment variables

| Variable | Value |
| -------- | ----- |
| `APP_ENV` | `production` |
| `BASE_URL` | `https://qrcode.binomargroup.com` |
| `SESSION_SECRET` | output of `openssl rand -base64 32` |
| `ADMIN_BOOTSTRAP_EMAIL` | `office@traveldoor.ge` |
| `ADMIN_BOOTSTRAP_PASSWORD` | a strong password, used once |
| `SEED_DEFAULT` | `true` |

`APP_ENV=production` refuses to start unless `BASE_URL` is `https://`, which is
a deliberate guard against generating QR codes that point at the wrong origin.

### 4. Domain and port

Add domain `qrcode.binomargroup.com`, **container port 8080**, HTTPS on,
certificate provider Let's Encrypt. Dokploy's Traefik terminates TLS and
forwards plain http to the container — correct, and why `Secure` cookies still
work.

### 5. Persistent volumes

Without these, every redeploy wipes the database and the uploaded logo:

| Mount path | Holds |
| ---------- | ----- |
| `/app/data` | SQLite database |
| `/app/uploads` | uploaded logos |

### 6. Deploy, then lock the admin down

Deploy, wait for healthy, then open
<https://qrcode.binomargroup.com/admin>, sign in with the bootstrap
credentials, and **change the password immediately** at
<https://qrcode.binomargroup.com/admin/account>. `ADMIN_BOOTSTRAP_*` is only
read when no user exists, so it does nothing after the first boot — but leaving
a known password on a public admin is the risk worth closing first.

### Deploying with the Compose service type

`docker-compose.yml` — the repository default — is written for this path: no host port, attached
to the external `dokploy-network`, with the Traefik routers, the TLS resolver
and the http-to-https redirect already declared for
`qrcode.binomargroup.com`.

1. **Create Service → Compose** in your Dokploy project, pointing at this
   repository.
2. Leave **Compose Path** at its default, `./docker-compose.yml`.
3. In the **Environment** tab set:

   ```
   SESSION_SECRET=<openssl rand -base64 32>
   ADMIN_BOOTSTRAP_PASSWORD=<a strong first-run password>
   ```

   Both use compose's `${VAR:?message}` form, so a missing value aborts the
   deploy with an explicit error rather than starting something insecure.
   `ADMIN_BOOTSTRAP_EMAIL` is optional and defaults to `office@traveldoor.ge`.
4. Deploy. Traefik picks up the labels; the certificate is issued on the first
   https request, so the DNS record has to resolve first.

The domain is baked into the compose file in two places — the two router rules
and `BASE_URL`. To serve a different hostname, change all three together:
`BASE_URL` is what the QR encodes, so a mismatch produces codes pointing at the
wrong origin.

Nothing in this file needs a Dokploy-side domain entry; the labels do the
routing. If you prefer to add the domain in the Dokploy UI instead, drop the
`labels:` block and let Dokploy generate its own.

Verified locally against a stand-in `dokploy-network`: the stack builds, comes
up healthy, seeds the profile, serves `https://qrcode.binomargroup.com` as its
canonical URL, exposes no host port, and carries exactly the labels above.

### Redeploys and your data

`SEED_DEFAULT=true` imports the bundled profile **only when its slug is
missing**, so redeploys never touch links, branding or ordering you have
changed in the admin. Use `SEED_DEFAULT=force` only when you deliberately want
the bundled definition to replace the live link set.

Back up before any risky change:

```bash
docker compose stop app && docker compose cp app:/app/data/app.db ./backup-app.db && docker compose start app
```

## Expose it publicly with Cloudflare Tunnel

A quick tunnel puts the container on the internet without opening a port:

```bash
cloudflared tunnel --url http://localhost:8090
```

It prints a `https://<random>.trycloudflare.com` hostname. **Restart the app
with that hostname as `QR_BASE_URL` before using it** — otherwise the QR code,
the canonical link and the Open Graph tags all still say `localhost`, which is
the usual reason a tunnelled instance looks broken:

```bash
QR_BASE_URL=https://your-name.trycloudflare.com QR_APP_ENV=production QR_SESSION_SECRET="$(openssl rand -base64 32)" docker compose -f docker-compose.local.yml up -d
```

`QR_APP_ENV=production` is right here because the tunnel terminates TLS: it
turns on `Secure` cookies and HSTS. The trade-off is that the app is then only
usable through the https hostname — signing in at `http://localhost:8090`
stops working, because the browser will not send a `Secure` cookie over plain
http.

**Change the admin password before you expose anything.** The bootstrap
password is in this README, so it is public knowledge:

```bash
docker compose -f docker-compose.local.yml exec app /app/app -set-password office@traveldoor.ge
```

It prompts on stdin; `-password <value>` is available for scripting but lands
in your shell history. On Git Bash for Windows, prefix the command with
`MSYS_NO_PATHCONV=1` or the `/app/app` path gets rewritten to a Windows path.

### Quick tunnels are temporary

Every `cloudflared tunnel --url` run gets a **different** hostname, and the
tunnel dies with the terminal. Since the QR encodes `BASE_URL/{slug}`, a code
generated against a `trycloudflare.com` name is worthless the moment the tunnel
restarts. Quick tunnels are for testing on a real phone — never print one.

For a QR you can actually print, use a named tunnel bound to your own
hostname (say `qr.traveldoor.ge`), then run with
`QR_BASE_URL=https://qr.traveldoor.ge`. That name stays stable, so the printed
code stays valid.

## Stack

Go (standard library `net/http`), server-rendered `html/template`, HTMX for
admin partial updates only, SQLite via the pure-Go `modernc.org/sqlite` driver,
one hand-written stylesheet. No SPA, no Node build step. Templates and static
assets are embedded in the binary.

## Run without Docker

```bash
cp .env.example .env
```

Then run:

```bash
go run ./cmd/app -seed-default
```

That creates `data/app.db`, applies the migrations, creates the admin user from
`ADMIN_BOOTSTRAP_*`, and imports the Travel Door profile. Start the
server:

```bash
go run ./cmd/app
```

Open <http://localhost:8080/traveldoor> for the public page and
<http://localhost:8080/admin> to sign in. If 8080 is taken, override both the
port and the canonical URL so the QR keeps matching:

```bash
ADDR=:8099 BASE_URL=http://localhost:8099 go run ./cmd/app
```

## Routes

| Method   | Route                            | Purpose                        |
| -------- | -------------------------------- | ------------------------------ |
| GET      | `/`                              | Redirects to the only published profile, otherwise lists them |
| GET      | `/{slug}`                        | Public profile page            |
| GET      | `/{slug}/contact.vcf`            | vCard download                 |
| GET      | `/go/{id}`                       | Click-counting redirect to a link destination |
| GET      | `/uploads/{file}`                | Uploaded profile logo          |
| GET      | `/healthz`                       | Health check                   |
| GET/POST | `/admin/login`                   | Sign in                        |
| POST     | `/admin/logout`                  | Sign out                       |
| GET      | `/admin`                         | Dashboard                      |
| GET      | `/admin/profiles/new`            | New profile form               |
| POST     | `/admin/profiles`                | Create profile                 |
| GET/POST | `/admin/profiles/{id}`           | View / update profile          |
| POST     | `/admin/profiles/{id}/publish`   | Publish or unpublish           |
| POST     | `/admin/profiles/{id}/delete`    | Delete profile                 |
| POST     | `/admin/profiles/{id}/links`     | Create link                    |
| POST     | `/admin/profiles/{id}/links/reorder` | Persist link ordering      |
| POST     | `/admin/links/{id}`              | Update link                    |
| POST     | `/admin/links/{id}/delete`       | Delete link                    |
| GET      | `/admin/profiles/{id}/qr.svg`    | QR as SVG (`?download=1`)      |
| GET      | `/admin/profiles/{id}/qr.png`    | QR as PNG (`?download=1&size=1024`) |
| GET      | `/admin/profiles/{id}/qr.jpg`    | QR as JPEG (`?download=1&size=1024`) |
| GET      | `/admin/profiles/{id}/qr.pdf`    | QR as a print-ready A4 PDF |
| GET      | `/admin/account`                 | Change the signed-in admin password |
| POST     | `/admin/account/password`        | Submit the password change |

## Project tree

```
cmd/app/main.go              entrypoint, config, graceful shutdown, seeding flags
embed.go                     embeds templates/ and static/
internal/auth/               bcrypt, cookie sessions, CSRF
internal/config/             .env + environment loading
internal/handlers/           routing, middleware, public + admin handlers
internal/models/             plain structs
internal/seed/               JSON profile import (bundled Travel Door data + logo)
internal/services/           validation, QR rendering, vCard
internal/store/              SQLite access
internal/store/migrations/   numbered .sql migrations, applied at startup
templates/layouts|public|admin|partials/
static/css|js/
uploads/                     logo uploads (served from disk)
Dockerfile                   two-stage static build, non-root runtime
docker-compose.yml           Dokploy/production stack (Traefik labels, no host port)
docker-compose.local.yml     local development stack (host port 8090)
```

## Configuration

Set in `.env` or the real environment (environment wins).

| Variable | Notes |
| -------- | ----- |
| `APP_ENV` | `development` or `production` |
| `ADDR` | listen address, default `:8080` |
| `BASE_URL` | canonical origin; **every QR encodes `BASE_URL/{slug}`**. Must be `https://` in production |
| `DATABASE_PATH` | SQLite file, default `data/app.db` |
| `SESSION_SECRET` | required in production — `openssl rand -base64 32` |
| `ADMIN_BOOTSTRAP_EMAIL` / `ADMIN_BOOTSTRAP_PASSWORD` | used only when no user exists yet |
| `UPLOAD_DIR` | logo upload directory |
| `SEED_DEFAULT` | `true` seeds only when the profile is missing; `force` re-imports every start, replacing the link set |

Changing `BASE_URL` after codes are printed invalidates them, exactly like
changing a slug.

## Seeding

```bash
go run ./cmd/app -seed-default          # bundled Travel Door profile
go run ./cmd/app -seed path/to/x.json   # any profile definition
```

To reset an admin password (prompts on stdin):

```bash
go run ./cmd/app -set-password office@traveldoor.ge
```

The import upserts by slug: re-running it refreshes name, contact details and
the whole link set while keeping the profile id and slug, so existing printed
QR codes keep resolving. The bundled definition lives at
`internal/seed/data/traveldoor.json`. Setting `SEED_DEFAULT=true` does the same
import at startup without exiting, which is how the container seeds itself.

## Link types and safety

Supported types: website, Facebook, Instagram, TikTok, YouTube, LinkedIn,
X/Twitter, WhatsApp, phone, email, map, generic.

* Destinations are normalised server-side: a phone number becomes `tel:`, an
  email becomes `mailto:`, a WhatsApp number becomes `https://wa.me/…` so there
  is always a web fallback when the app is not installed.
* Only `http`, `https`, `mailto` and `tel` are accepted. `javascript:`,
  `data:`, `vbscript:` and `file:` are rejected on save **and** re-checked
  before any redirect.
* `http(s)` links render through `/go/{id}` for click counting; `tel:` and
  `mailto:` render directly so they work with JavaScript disabled.

## Public page details

* Links render as an app-launcher grid: a large platform mark over a small
  caption, with the tile border in that platform's own colour.
* Brand marks are the official [Simple Icons](https://simpleicons.org) glyphs
  (CC0), embedded as inline SVG paths. Website, email, phone and map have no
  official mark and use hand-drawn glyphs. TikTok and X are near-black, so the
  dark theme inverts those two.
* "Share link" uses the Web Share API where the browser has one (the native
  sheet on phones) and falls back to copying the URL to the clipboard. The
  button ships with the `hidden` attribute and is revealed by script, so a
  browser without JavaScript never sees an action it cannot perform.
* The logo sits on a solid `#264a90` plaque because the supplied mark is a
  reversed (white) wordmark that would otherwise vanish on the light card.

## Analytics and privacy

`events` stores page views and link clicks with a timestamp and, at most, the
referrer **hostname**. No IP addresses, user agents, cookies or visitor
identifiers are recorded, so the counts are aggregate-only.

## Security baseline

* bcrypt password hashing; login timing does not reveal whether an email exists.
* Server-side sessions; cookies are `HttpOnly`, `SameSite=Lax`, and `Secure`
  when `APP_ENV=production`.
* Double-submit CSRF token required on every authenticated mutation.
* All SQL is parameterised; all output is escaped by `html/template`.
* Response headers: `Content-Security-Policy` (no inline scripts),
  `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, plus HSTS in
  production.
* Admin pages are `noindex, nofollow`, and unpublished profiles return 404.
* Logo uploads are capped at 2 MB, sniffed for a real raster image type (SVG
  is refused because it can carry script), stored under a filename derived
  from the profile id, and served from a flat directory only.

Run behind an HTTPS reverse proxy (Caddy, nginx, Traefik) in production.

## Tests

```bash
go test ./...
```

Covers URL/scheme validation and normalisation, slug rules, published vs.
unpublished slug lookup, disabled links disappearing from the page and stopping
their redirect, link ordering (both the explicit order list and single-step moves), CSRF
enforcement, admin auth, QR determinism and stability across content edits,
vCard contents, and idempotent seeding.

## Build and deploy

```bash
go build -trimpath -ldflags="-s -w" -o app ./cmd/app
```

The binary is self-contained apart from `data/` and `uploads/`.

The image is a two-stage build: `CGO_ENABLED=0` static binary on
`golang:1.26-alpine`, running as a non-root user on `alpine:3.22` with a
`/healthz` healthcheck. For a real deployment, set `QR_APP_ENV=production`, a
real `QR_SESSION_SECRET` and an `https://` `QR_BASE_URL`, and put the container
behind a TLS-terminating reverse proxy.

## Backup and restore

SQLite runs in WAL mode, so copy with the SQLite backup API rather than `cp`:

```bash
sqlite3 data/app.db ".backup 'backup/app-$(date +%F).db'"
```

Under Docker the database lives in the `data` volume. Stop the container first
so the WAL is checkpointed, then copy it out:

```bash
docker compose stop app && docker compose cp app:/app/data/app.db ./backup-app.db && docker compose start app
```

Also copy `uploads/`. To restore, stop the app, put the backup at
`DATABASE_PATH`, remove any stale `app.db-wal` / `app.db-shm`, start the app,
and verify: sign in, load `/{slug}`, and confirm the QR still resolves. Test
the restore in a separate environment before relying on it.

## Reprinting rule

Reprint the QR only when the slug or `BASE_URL` changes. Edits to name,
subtitle, logo, links, ordering or publish state never change the encoded URL —
`TestQRStaysStableWhenContentChanges` enforces this.
# traveldoor_community_QR_code
