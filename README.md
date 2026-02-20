# Self-Hosted Health Dashboard

A lightweight, self-hosted health dashboard for indie apps. Single Go binary with SQLite, no external dependencies.

## Features

- **Uptime monitoring** — HTTP checks with configurable intervals and alerting
- **System metrics** — CPU, memory, and disk tracking via a companion agent binary
- **Business events** — Lightweight event ingestion API for tracking signups, conversions, etc.
- **Single-user auth** — Session-based login with bcrypt password hashing
- **7-day retention** — Automatic pruning of old data

## Quick Start

### Docker

```bash
docker build -t health-dashboard .
docker run -p 8080:8080 -v $(pwd)/data:/data health-dashboard
```

Open `http://localhost:8080` and log in with the password from `config.yaml`.

### From Source

```bash
go build -o server ./cmd/server
go build -o agent ./cmd/agent
./server --config config.yaml
```

## Configuration

Copy and edit `config.yaml`:

```yaml
server:
  port: 8080
  data_dir: /data

auth:
  password: "changeme"        # use --hash-password to generate a bcrypt hash
  session_secret: "..."

agent:
  token: "..."                # shared token for the agent binary
  server_url: "http://localhost:8080"

alerts:
  webhook_url: ""             # optional — POST on monitor-down events

events:
  api_key: "..."              # X-API-Key for event ingestion
```

Hash a password for production:

```bash
./server --hash-password 'mysecretpassword'
```

Paste the output into `config.yaml` as `auth.password`.

## System Agent

Run the agent binary on each host you want to monitor:

```bash
./agent --config config.yaml
```

The agent reads `/proc/stat`, `/proc/meminfo`, and `/proc/diskstats` every 30 seconds and posts metrics to the server.

## Business Event Ingestion API

Track custom events (signups, conversions, payments, etc.) with a simple HTTP call.

### Post an event

```bash
curl -X POST http://localhost:8080/api/events \
  -H "X-API-Key: your-events-api-key" \
  -H "Content-Type: application/json" \
  -d '{"event_name": "signup"}'
```

The `value` field is optional and defaults to `1`. To record a revenue event:

```bash
curl -X POST http://localhost:8080/api/events \
  -H "X-API-Key: your-events-api-key" \
  -H "Content-Type: application/json" \
  -d '{"event_name": "revenue", "value": 49.99}'
```

### Get event summary

```bash
curl http://localhost:8080/api/events/summary \
  -H "X-API-Key: your-events-api-key"
```

Returns per-event totals for today and the trailing 7 days:

```json
[
  {"event_name": "revenue",  "today": 149.97, "last_7_days": 749.85},
  {"event_name": "signup",   "today": 3,      "last_7_days": 21}
]
```

## Uptime Monitor API

```bash
# Create a monitor
curl -X POST http://localhost:8080/api/monitors \
  -H "Content-Type: application/json" \
  -b "session=<token>" \
  -d '{"name":"My App","url":"https://example.com","interval_seconds":60}'

# List monitors
curl http://localhost:8080/api/monitors -b "session=<token>"

# Recent checks for a monitor
curl http://localhost:8080/api/monitors/1/checks -b "session=<token>"
```

## Data Retention

All data is automatically pruned to 7 days:
- Uptime checks
- System metrics
- Business events
