# Scout - CRE Site-Selection Platform

A geospatial platform for Commercial Real Estate professionals to identify and rank developable parcels by overlaying constraints and infrastructure proximity.

## Architecture

- **API Service**: Go/chi-based REST API gateway
- **Worker Service**: Background job processing (exports, tiling, feature computation)
- **Database**: PostgreSQL + PostGIS for geospatial data
- **Search**: OpenSearch
- **Queue**: Valkey or Postgres-backed jobs
- **Storage**: S3-compatible object storage for tiles and exports

## Quick Start

### Prerequisites

- Go 1.25+
- PostgreSQL 17+ with PostGIS extension
- Valkey (optional, for job queue)
- OpenSearch (optional, for search functionality)
- Docker & Docker Compose (optional, for local development)

### Local Development

1. **Set up environment variables:**

   Create a `.env` file based on `.envrc` or configure the following variables:
   - Database connection (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME)
   - API configuration (API_PORT, JWT_SECRET)
   - Optional: Valkey, OpenSearch, S3 credentials

2. **Start dependencies (Docker):**

   ```bash
   make docker-up
   ```

   This starts:
   - PostgreSQL with PostGIS (port 5432)
   - Valkey (port 6379)
   - OpenSearch (port 9200)
   - OpenSearch Dashboards (port 5601)

3. **Run database migrations:**

   ```bash
   make migrate-up
   ```

4. **Run the API service:**

   ```bash
   make run-api
   ```

   The API will be available at `http://localhost:8080`

5. **Run the worker service (in another terminal):**

   ```bash
   make run-worker
   ```

### Building

```bash
make build
```

Binaries will be in `bin/` directory.

### Testing

```bash
make test              # Run tests
make test-coverage     # Run tests with coverage report
make test-race         # Run tests with race detector
```

## Project Structure

```text
.
├── cmd/
│   ├── scout/        # Main CLI entry point (Cobra commands)
│   ├── api/          # Standalone API service entry point
│   └── worker/       # Standalone worker service entry point
├── internal/
│   ├── api/          # API handlers and routes
│   ├── worker/       # Worker job handlers
│   ├── models/       # Data models
│   ├── database/     # Database connection and migrations
│   ├── config/       # Configuration management
│   ├── middleware/   # HTTP middleware (auth, logging, etc.)
│   ├── search/       # Search service abstraction
│   ├── storage/      # Object storage client
│   └── scoring/      # Suitability scoring logic
├── migrations/       # Database migrations
├── docs/             # Documentation (Swagger specs)
└── bin/              # Build output directory
```

## API Documentation

API documentation is available via Swagger UI:

- Swagger UI: `http://localhost:8080/swagger/index.html`
- Swagger JSON: `http://localhost:8080/swagger/doc.json`

### Core Endpoints

- `POST /api/suitability/search` - Search parcels with filters
- `GET /api/parcels/{parcelId}` - Get parcel details
- `POST /api/exports` - Create export job
- `GET /tiles/{layer}/{z}/{x}/{y}.pbf` - Vector tiles
- `GET /health` - Health check endpoint

## Development

### Adding a Migration

```bash
make migrate-create NAME=add_parcels_table
```

### Code Quality

```bash
make fmt      # Format code
make lint     # Run linter (requires golangci-lint)
make vet      # Run go vet
make tidy     # Tidy and verify dependencies
```

### Docker Commands

```bash
make docker-up      # Start containers
make docker-down    # Stop containers
make docker-logs    # View container logs
make docker-build   # Build Docker images
```

## License

This project is licensed under the GNU Affero General Public License v3.0 (AGPLv3).

See the [LICENSE](LICENSE) file for details.
