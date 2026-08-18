# QR Social Profile Platform

## Purpose
One stable QR code opens a fast, branded business profile page containing social media, contact and business links.

## Recommended MVP stack
- Go
- Server-rendered HTML templates
- HTMX for admin interactions
- SQLite
- Minimal CSS and JavaScript

## Core URLs
- `/{slug}` public profile
- `/{slug}/contact.vcf` contact card
- `/admin` administration

## Core principle
The QR stores the stable public profile URL. Content can change without replacing the printed QR code.

## Local configuration
Create `.env` from `.env.example` and configure:
- `APP_ENV`
- `BASE_URL`
- `DATABASE_PATH`
- `SESSION_SECRET`
- `ADMIN_BOOTSTRAP_EMAIL`
- `ADMIN_BOOTSTRAP_PASSWORD`

## Security baseline
- HTTPS in production
- secure password hashing
- secure cookies
- CSRF protection
- server-side validation
- reject unsafe URL schemes
- HTML escaping
- rate-limit login attempts if exposed publicly

## Backup
Stop or safely checkpoint the database, copy the SQLite database file, and verify restoration in a separate environment.

## Performance
Public pages should remain server-rendered and cache-friendly. Avoid SPA bundles and unnecessary client-side frameworks.
