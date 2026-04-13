# JobHuntly Job Data App

A full-stack job search app with:

- Go backend API
- Astro frontend
- SQLite persistence
- live ingestion from job APIs
- optional external and built-in scraping
- cookie-based user sessions
- bookmarks and analytics

The app is packaged to run through a **single Docker container**. The container starts the Astro frontend internally and exposes the whole product through the Go server on port `8080`.

## Reference Documentation

For a full project map covering architecture, packages, data flow, storage, screens, endpoints, integrations, and current implementation notes, see [docs/project-reference.md](docs/project-reference.md).

## What Works

- Search jobs by role
- Filter jobs by employment type, experience, source, and remote-only
- Save and unsave jobs
- Register and log in with server-backed session cookies
- Browse companies
- View analytics and source health
- Ingest jobs from JSearch and Adzuna
- Optionally ingest from an external scrape bridge
- Optionally enable experimental built-in HTML scraping

## Tech Stack

### Backend

- Go
- chi router
- SQLite via `modernc.org/sqlite`
- JWT-backed `HttpOnly` cookie sessions
- sqlx

### Frontend

- Astro
- Preact islands
- Tailwind CSS

## Local Development

### Prerequisites

- Go 1.25+
- Node.js 20+

### 1. Configure environment

```bash
cp .env.example .env
```

Edit `.env` with any API keys you want to use.

The backend auto-loads `.env` for local runs.

### 2. Run the backend

```bash
go run ./cmd/server
```

The API will be available at [http://localhost:8080](http://localhost:8080).

### 3. Run the frontend

```bash
cd frontend
npm install
npm run dev
```

The frontend will be available at [http://localhost:4321](http://localhost:4321).

## Single-Container Docker Run

### Build the image

```bash
docker build -t jobhuntly-app .
```

### Run the image

```bash
docker run --env-file .env -p 8080:8080 -v jobhuntly-data:/data jobhuntly-app
```

Open [http://localhost:8080](http://localhost:8080).

The single container will:

1. start Astro internally on `127.0.0.1:4321`
2. start the Go server on `0.0.0.0:8080`
3. proxy all frontend routes through the Go server
4. keep SQLite data in `/data/jobpulse.db`

## Docker Compose

`docker-compose.yml` now runs the app as a **single service**:

```bash
docker compose up --build
```

Then open [http://localhost:8080](http://localhost:8080).

## Environment Variables

### Core

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Public server port |
| `DATABASE_PATH` | `jobpulse.db` | SQLite database path |
| `JWT_SECRET` | dev default | JWT signing secret; set a strong value in production |
| `CORS_ORIGINS` | local defaults | Allowed origins |
| `FRONTEND_SERVER_URL` | empty | Normally set automatically by `start.sh` inside the Docker image |
| `SOURCE_FETCH_WORKERS` | `4` | Max concurrent source fetch workers in the backend aggregator |

### Rate limiting

| Variable | Default | Description |
|---|---|---|
| `RATE_LIMIT_ENABLED` | `true` | Enables in-memory API rate limiting |
| `RATE_LIMIT_REQUESTS` | `120` | Request budget per `RATE_LIMIT_WINDOW` for general API traffic |
| `RATE_LIMIT_WINDOW` | `1m` | Fixed window used by the general API limiter |
| `AUTH_RATE_LIMIT_REQUESTS` | `20` | Request budget per auth window for `/api/v1/auth/*` |
| `AUTH_RATE_LIMIT_WINDOW` | `1m` | Fixed window used by auth endpoints |
| `ADMIN_RATE_LIMIT_REQUESTS` | `10` | Request budget per admin window for `/api/v1/admin/*` |
| `ADMIN_RATE_LIMIT_WINDOW` | `1m` | Fixed window used by admin endpoints |

### API ingestion

| Variable | Description |
|---|---|
| `JSEARCH_API_KEY` | RapidAPI JSearch key |
| `ADZUNA_APP_ID` | Adzuna app id |
| `ADZUNA_APP_KEY` | Adzuna app key |

### Background live sync

| Variable | Example | Description |
|---|---|---|
| `LIVE_SYNC_QUERIES` | `golang developer|python developer` | Queries refreshed in the background |
| `LIVE_SYNC_LOCATIONS` | `Remote|San Francisco, CA` | Optional location list for background sync |
| `LIVE_SYNC_INTERVAL` | `30m` | Sync cadence |
| `LIVE_SYNC_ON_START` | `true` | Run one sync on startup |

### External scrape bridge

| Variable | Description |
|---|---|
| `SCRAPE_BRIDGE_URL` | External scraping worker endpoint |
| `SCRAPE_BRIDGE_TOKEN` | Optional bearer token for the bridge |
| `SCRAPE_BRIDGE_SOURCES` | Comma- or pipe-separated sources, for example `linkedin,indeed` |

### Experimental built-in scrapers

These are **optional** and should be treated as best-effort ingestion, not as your most reliable production source.

| Variable | Default | Description |
|---|---|---|
| `ENABLE_BUILTIN_SCRAPERS` | `false` | Enables built-in HTML scraping |
| `BUILTIN_SCRAPER_SOURCES` | `linkedin,indeed` | Scraper providers to enable |

## Ingestion Notes

### API ingestion

The app can ingest directly from:

- JSearch
- Adzuna

Search requests with a role query trigger a live refresh before reading from local storage. Freshly fetched jobs are stored in SQLite and also returned directly if the local DB query layer misses them.

### Built-in scraping

The app now includes an **experimental** built-in scraping source for LinkedIn-style and Indeed-style public HTML pages.

Important:

- these pages can change without notice
- rate limits and anti-bot protections can break scraping
- API sources are still more stable than scraping

If you need stronger scraping reliability, prefer the external scrape bridge path.

## Authentication and Sessions

- Registration and login are implemented
- Sessions are stored in an `HttpOnly` cookie
- Logout clears the cookie
- Saved jobs and profile routes use the cookie session

There is **no email verification** and **no password reset flow** in the current app.

## API Endpoints

### Public

| Endpoint | Description |
|---|---|
| `GET /health` | Health check |
| `GET /api/v1/jobs` | List jobs |
| `GET /api/v1/jobs/{id}` | Get job details |
| `GET /api/v1/companies` | List companies |
| `GET /api/v1/companies/{slug}` | Get company details |
| `GET /api/v1/filters` | Job filter options |
| `GET /api/v1/analytics/summary` | Analytics summary |
| `GET /api/v1/analytics/skills` | Top skills |
| `GET /api/v1/analytics/trends` | Market trends |
| `GET /api/v1/analytics/sources` | Source distribution |
| `GET /api/v1/analytics/source-health` | Source health status |
| `GET /api/v1/analytics/salary` | Salary statistics |
| `POST /api/v1/analytics/refresh` | Refresh analytics snapshots |
| `POST /api/v1/auth/register` | Register |
| `POST /api/v1/auth/login` | Login |
| `POST /api/v1/auth/logout` | Logout |
| `GET /api/v1/auth/session` | Current session status |

### Authenticated

| Endpoint | Description |
|---|---|
| `GET /api/v1/me` | Current user profile |
| `GET /api/v1/me/saved-jobs` | Saved jobs |
| `POST /api/v1/me/saved-jobs/{id}` | Save a job |
| `DELETE /api/v1/me/saved-jobs/{id}` | Unsave a job |

## Run Tests

### Backend

```bash
go test ./...
```

### Frontend build verification

```bash
cd frontend
npm run build
```

## Project Structure

```text
job-data-scraper/
├── cmd/server/
├── internal/
│   ├── api/
│   ├── config/
│   ├── database/
│   ├── domain/
│   ├── repository/
│   ├── service/
│   └── sources/
│       ├── adzuna/
│       ├── jsearch/
│       ├── scrapebridge/
│       └── webscrape/
├── frontend/
├── Dockerfile
├── docker-compose.yml
├── start.sh
└── README.md
```

## Production Notes

Recommended for real deployment:

- set a strong `JWT_SECRET`
- mount `/data` as a persistent volume
- provide real API credentials
- treat built-in scraping as optional, not primary
- monitor `/api/v1/analytics/source-health`

## License

MIT
