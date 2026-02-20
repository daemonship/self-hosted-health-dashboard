# Self-Hosted Health Dashboard

A lightweight, self-hosted health dashboard for indie apps. Single Go binary with SQLite, no external dependencies.

## Status

> 🚧 In active development — not yet production ready

| Feature | Status | Notes |
|---------|--------|-------|
| Project scaffold & CI | ✅ Complete | Go 1.22, SQLite WAL, Docker |
| Auth & health endpoint | ✅ Complete | bcrypt, session cookies |
| Uptime monitor CRUD API | ✅ Complete | HTTP checker, configurable intervals |
| System agent binary | ✅ Complete | CPU/mem/disk via /proc, 30 s interval |
| Business event ingestion | ✅ Complete | POST /api/events, X-API-Key auth |
| Unified dashboard frontend | ✅ Complete | Preact + uPlot, 3-section layout, 30 s refresh |
| Webhook alerting | ✅ Complete | Monitor-down POST, retry once after 5 s |
| Code review | 🚧 In Progress | |

## Features

- **Unified dashboard** — Three-section Preact UI: uptime monitors, system metrics gauges + time-series chart, and business event counters. Auto-refreshes every 30 s.
- **Uptime monitoring** — HTTP checks with configurable intervals; 24-hour uptime % visible at a glance
- **System metrics** — CPU, memory, and disk tracking via a companion agent binary; 24 h history charted with uPlot
- **Business events** — Lightweight event ingestion API for tracking signups, conversions, etc.
- **Webhook alerting** — POST notification when a monitor transitions to down; retries once on failure
- **Single-user auth** — Session-based login with bcrypt password hashing
- **7-day retention** — Automatic pruning of old data; all JS/CSS bundled offline (no CDN at runtime)

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

## Webhook Alerting

Set `alerts.webhook_url` in `config.yaml` to receive a POST request whenever a monitor transitions to **down** (after 3 consecutive failures).

**Payload:**

```json
{
  "monitor_name": "My App",
  "url":          "https://example.com",
  "status":       "down",
  "timestamp":    "2026-02-19T12:34:56Z"
}
```

The webhook is fired once on the state transition. If the POST fails, it retries once after 5 seconds. Attempts are logged to stdout. There is no alert history UI — check your webhook receiver or server logs.

**Example — send to a Slack-compatible endpoint:**

```bash
alerts:
  webhook_url: "https://hooks.slack.com/services/T000/B000/xxxx"
```

Leave `webhook_url` empty (the default) to disable alerting.

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
