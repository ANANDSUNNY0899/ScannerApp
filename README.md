# ?? ScannerApp Cloud Backend

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Docker](https://img.shields.io/badge/Docker-Multi--Stage-2496ED?style=flat&logo=docker)](Dockerfile)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-4169E1?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-RateLimiting-DC382D?style=flat&logo=redis)](https://redis.io/)
[![MinIO/S3](https://img.shields.io/badge/Storage-S3%20%2F%20R2-FF9900?style=flat&logo=amazons3)](https://min.io/)
[![Deploy to Render](https://img.shields.io/badge/Deploy%20to-Render-46E3B7?style=flat&logo=render)](https://render.com)

High-performance, offline-first cloud synchronization and OCR backend for the **Document Scanner** mobile ecosystem (Android Jetpack Compose client). Built in Go with PostgreSQL, Redis, S3/MinIO/Cloudflare R2 object storage, and Google Play Billing server-to-server verification.

---

## ?? Table of Contents

- [Architecture Overview](#-architecture-overview)
- [Key Features](#-key-features)
- [Tech Stack](#-tech-stack)
- [API Reference](#-api-reference)
- [Environment Variables](#-environment-variables)
- [Database Migrations](#-database-migrations)
- [Local Development](#-local-development)
- [Production Deployment (Render)](#-production-deployment-render)
- [Security & Hardening](#-security--hardening)

---

## ?? Architecture Overview

```text
+--------------------------------------------------------+
¦               Android Client (Jetpack Compose)         ¦
¦          Room DB (Offline First) + Google ML Kit       ¦
+--------------------------------------------------------+
                            ¦ HTTPS / REST (JWT Auth)
                            ?
+--------------------------------------------------------+
¦                   Go API Server (Render)               ¦
¦                                                        ¦
¦  +--------------+   +---------------+  +-------------+ ¦
¦  ¦ JWT / Bcrypt ¦   ¦ LWW Sync Push ¦  ¦ Play Billing¦ ¦
¦  ¦   Auth API   ¦   ¦  & Pull Engine¦  ¦ Verification¦ ¦
¦  +--------------+   +---------------+  +-------------+ ¦
¦         ¦                   ¦                 ¦        ¦
+---------+-------------------+-----------------+--------+
          ¦                   ¦                 ¦
    +-----?-------+     +-----?-------+   +-----?-------+
    ¦ PostgreSQL  ¦     ¦    Redis    ¦   ¦  S3 / R2 /  ¦
    ¦  (Metadata, ¦     ¦  (Atomic OCR¦   ¦    MinIO    ¦
    ¦ Tombstones, ¦     ¦ Rate Limits ¦   ¦ (Scans/PDFs ¦
    ¦ LWW Timests)¦     ¦ & Sessions) ¦   ¦ Presigned)  ¦
    +-------------+     +-------------+   +-------------+
```

---

## ? Key Features

1. **Bi-Directional Sync with Last-Write-Wins (LWW)**:
   - Synchronizes folders, documents, and individual page metadata.
   - Client-generated RFC3339 timestamps (`client_updated_at`) resolve multi-device conflicts.
   - Soft deletes (tombstones) propagate across devices with cascade disk cleanup.

2. **S3-Compatible Object Storage**:
   - Direct streaming uploads for scanned page images and multi-page PDFs.
   - Generates presigned URLs with 2-hour expiration for secure downloads.
   - Supports **MinIO**, **Cloudflare R2**, and **AWS S3**.

3. **Atomic Rate Limiting & Cloud OCR**:
   - Redis-backed sliding window counter limiting free-tier users to 5 requests.
   - Rate limit check is completely bypassed for verified **Premium** subscribers.

4. **Google Play Billing Verification**:
   - Server-to-server purchase validation using the Google Android Publisher API v3 (`Subscriptionsv2.Get`).
   - Upgrades account to `premium` in PostgreSQL upon valid purchase token confirmation.
   - Includes automatic sandbox/mock fallback mode for local emulator testing.

5. **Production Multi-Stage Docker Build**:
   - `golang:1.22-alpine` builder compiles a stripped, static binary.
   - `alpine:latest` runner image with system CA certificates and bundled SQL migrations.
   - Dynamically binds to `$PORT` assigned by cloud providers.

6. **Automatic Startup Migrations**:
   - Automatically applies all `*.up.sql` migrations in alphanumeric order when PostgreSQL connects.

---

## ?? Tech Stack

- **Language**: Go 1.22+
- **Router**: `gorilla/mux`
- **Database**: PostgreSQL 16+ with `sqlx` and `lib/pq`
- **Cache & Quotas**: Redis with `go-redis/v9`
- **Object Storage**: MinIO Go SDK (`minio-go/v7`) compatible with AWS S3 & Cloudflare R2
- **Authentication**: JWT (`golang-jwt/jwt/v5`) with `golang.org/x/crypto/bcrypt`
- **Google Play APIs**: `google.golang.org/api/androidpublisher/v3`

---

## ?? API Reference

All protected endpoints require an `Authorization: Bearer <JWT>` header.

### 1. Authentication (`/api/v1/auth`)

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/auth/register` | Register a new user account | No |
| `POST` | `/api/v1/auth/login` | Authenticate user & receive JWT token | No |
| `GET` | `/api/v1/auth/me` | Fetch authenticated user profile and tier | **Yes** |

### 2. Synchronization (`/api/v1/sync`)

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/sync/push` | Push batched folders, documents, and pages (LWW) | **Yes** |
| `GET` | `/api/v1/sync/pull?since=<ts>` | Pull remote updates & tombstones since timestamp | **Yes** |

### 3. Documents & Files (`/api/v1/documents`)

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/documents/{id}/file` | Multipart upload for document PDF/scan file to S3 | **Yes** |
| `GET` | `/api/v1/documents/{id}` | Retrieve document metadata & presigned S3 URL | **Yes** |

### 4. Cloud OCR & Quotas (`/api/v1/ocr`)

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/ocr/process` | Extract text from image (enforces 5-request limit) | **Yes** |
| `GET` | `/api/v1/ocr/quota` | Get remaining free OCR requests from Redis | **Yes** |

### 5. In-App Purchases & Subscriptions (`/api/v1/subscriptions`)

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/subscriptions/verify` | Verify Google Play purchase token & upgrade user | **Yes** |

---

## ?? Environment Variables

| Variable | Default (Local) | Description |
| :--- | :--- | :--- |
| `PORT` | `8080` | Server HTTP port (automatically set by Render) |
| `DATABASE_URL` | `postgres://...` | PostgreSQL connection string |
| `REDIS_ADDR` | `localhost:6379` | Redis host:port |
| `REDIS_PASSWORD` | `""` | Redis authentication password |
| `JWT_SECRET` | `super-secret-...` | Secret key for signing JWT tokens |
| `JWT_EXPIRATION_HOURS`| `72` | Token validity period in hours |
| `MAX_FREE_OCR` | `5` | Maximum free OCR extractions per user |
| `S3_ENDPOINT` | `localhost:9000` | S3 / MinIO / Cloudflare R2 endpoint host |
| `S3_ACCESS_KEY` | `minioadmin` | S3 / R2 Access Key |
| `S3_SECRET_KEY` | `minioadmin` | S3 / R2 Secret Key |
| `S3_BUCKET` | `scans-bucket` | Storage bucket name |
| `S3_USE_SSL` | `false` | Set to `true` for Cloudflare R2 or AWS S3 |
| `GOOGLE_APPLICATION_CREDENTIALS` | `""` | Path to Google Service Account JSON |

---

## ?? Database Migrations

Migrations are stored in [`migrations/`](migrations/) and executed automatically on startup:

- `000001_init_schema.up.sql`: Users, folders, documents, pages schema.
- `000002_add_sync_timestamps.up.sql`: Adds `client_updated_at` and `deleted_at` tombstones.
- `000003_add_user_tier.up.sql`: Adds `tier` column for free/premium user management.

All migrations are completely idempotent (`IF NOT EXISTS`).

---

## ?? Local Development

### 1. Prerequisites
- Go 1.22+
- Docker & Docker Compose

### 2. Run Infrastructure with Docker Compose
```bash
# Start PostgreSQL, Redis, and MinIO
docker compose up -d postgres redis minio minio-init
```

### 3. Run Backend API Server
```bash
go run ./cmd/server
```

### 4. Run Tests
```bash
go test -v ./...
```

---

## ?? Production Deployment (Render)

### Step 1: Create Managed PostgreSQL & Redis
1. In the [Render Dashboard](https://dashboard.render.com), create a **PostgreSQL** database.
2. Create a **Redis** instance.
3. Copy their internal connection URLs.

### Step 2: Create Web Service
1. Click **New +** -> **Web Service**.
2. Connect this repository: `ANANDSUNNY0899/ScannerApp`.
3. Set the following settings:
   - **Runtime**: `Docker`
   - **Branch**: `main`
   - **Region**: Select closest to your users.
4. Add the **Environment Variables** listed in the [Environment Variables](#-environment-variables) section.
5. Click **Create Web Service**.

Render will automatically build the multi-stage Dockerfile, apply database migrations on boot, and expose your service on a public HTTPS URL.

---

## ?? Security & Hardening

- **Zero Secrets Committed**: All secrets, environment configs (`*.env`), and credentials are strictly ignored via [`.gitignore`](.gitignore).
- **Password Hashing**: Bcrypt with salt rounds for secure storage.
- **JWT Protection**: Cryptographically signed tokens verified on every protected route.
- **TLS Security**: CA certificates bundled in the Alpine runner image ensure all outbound calls to R2 and Google APIs use verified HTTPS.
- **Rate Limit Resilience**: Redis atomic operations prevent race conditions and quota abuse.

---

## ?? License

This project is licensed under the MIT License.
