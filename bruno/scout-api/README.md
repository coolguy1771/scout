# Scout API - Bruno Collection

This Bruno collection contains API requests for the Scout CRE Site-Selection Platform.

## Setup

1. **Install Bruno**: Download from [https://www.usebruno.com/](https://www.usebruno.com/)

2. **Open Collection**:
   - Open Bruno
   - Click "Open Collection"
   - Select the `bruno/scout-api` directory

3. **Set Environment Variables**:
   - Select the "local" environment
   - Set the `token` variable to your JWT token
   - You can generate a token using: `./bin/scout token`

## Generating a Token

First, bootstrap test data (if not already done):

```bash
./bin/scout bootstrap
```

Then generate a token with the user and tenant IDs:

```bash
./bin/scout token --user-id <user-id> --tenant-id <tenant-id> --role admin
```

Copy the token and paste it into the Bruno environment variable `token`.

## Endpoints

### Health

- **Health Check** - Simple health endpoint (no auth required)

### Projects

- **Create Project** - Create a new project
- **List Projects** - List all projects for tenant
- **Get Project** - Get project details
- **Update Project** - Update project name
- **Delete Project** - Delete a project

### Search

- **Search Parcels** - Two-phase parcel search with scoring

### Parcels

- **Get Parcel** - Get parcel details with features
- **Get Nearby Features** - Find nearby infrastructure

### Saved Searches

- **Create Saved Search** - Save a search query
- **List Saved Searches** - List all saved searches
- **Run Saved Search** - Execute a saved search

### Exports

- **Create Export** - Create an export job (CSV, GeoJSON, PDF)
- **Get Export** - Get export metadata and download URL

### Jobs

- **Get Job Status** - Check async job status

### Tiles

- **Get Tile** - Get vector tile (PBF format)

## Notes

- Most endpoints require authentication via Bearer token
- Replace path parameters (like `{{projectId}}`) with actual UUIDs
- The API runs on `http://localhost:8080` by default
- Empty arrays are returned as `[]` (not `null`)
