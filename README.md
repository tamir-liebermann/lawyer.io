# lawyer.io — Israeli Real Estate Law Office AI (MVP)

Go backend + RTL Hebrew chat UI for an Israeli real estate law office.
Uses the Anthropic Messages API (Claude) for the LLM and the public
data.gov.il datastore for market data.

> תחילה — שירות מידע בלבד, אינו מהווה ייעוץ משפטי מחייב.

## Quick start

```bash
# 1. Install deps (populates go.sum)
go mod tidy

# 2. Set env vars
cp .env.example .env
# edit .env and set ANTHROPIC_API_KEY + SESSION_SECRET
export $(grep -v '^#' .env | xargs)

# 3. Run
go run ./cmd/server/main.go
# serves on http://localhost:8080
```

Open `http://localhost:8080` in a browser. Add `?mode=lawyer` to enter
lawyer mode directly, or click "צוות המשרד" in the sidebar.

## Project layout

```
cmd/server/main.go              # HTTP entry point, route wiring, graceful shutdown
internal/anthropic/             # Minimal Claude Messages API client + system prompt
internal/chat/                  # HTTP handler, gorilla/sessions integration
internal/realestatedata/        # data.gov.il fetcher + Hebrew summary formatter
internal/forms/                 # Static definitions for Israeli gov forms (7002 / 7000 / tabu)
web/static/                     # RTL Hebrew frontend: index.html, app.js, style.css
```

## API

All endpoints accept/return JSON. The frontend must use `credentials: same-origin`
so the session cookie (`lawyer_session`) roundtrips.

| Method | Path | Body / Query | Notes |
|---|---|---|---|
| `POST` | `/api/chat` | `{"message": "...", "mode": "client"\|"lawyer"}` or `?mode=lawyer` | Main chat endpoint. Returns `{reply, session_id, user_type, suggested_actions}`. |
| `POST` | `/api/mode` | `{"mode": "client"\|"lawyer"}` | Set the session's user type. |
| `POST` | `/api/reset` | (none) | Clear chat history for the current session. |
| `GET`  | `/api/forms` | (none) | Supported government forms + fields. |
| `GET`  | `/api/realestate?city=תל+אביב` | — | Direct data.gov.il summary. |
| `GET`  | `/healthz` | — | Liveness probe. |

## Running the tests

```bash
go test ./...
# verbose
go test -v ./...
# race detector (recommended)
go test -race ./...
# coverage report
go test -cover ./...
```

All tests are hermetic — they never call out to the real Anthropic API or
data.gov.il. Each package spins up an `httptest.Server` to imitate the upstream
API where needed.

### Test inventory

| Package | Test file | Covers |
|---|---|---|
| `internal/anthropic` | `client_test.go` | Messages API client; header/model/body shape; error handling; multi-block text joining; `BuildSystemPrompt` addendum selection + real-estate context injection. |
| `internal/chat` | `session_test.go` | Session cookie roundtrip, user-type validation, history append/trim/reset, copy isolation. |
| `internal/chat` | `handler_test.go` | POST /api/chat happy path, mode persistence (body + URL param), empty/invalid/oversized body rejection, LLM error mapping (503/502), real-estate context injection, suggested_actions, reset + mode endpoints, city detection heuristic. |
| `internal/chat` | `integration_test.go` | End-to-end: real Handler + SessionStore + real Anthropic client pointed at a fake upstream; cookies roundtrip across turns. |
| `internal/realestatedata` | `fetcher_test.go` | data.gov.il query shape; city / date / zero-price filtering; alternate field-name normalization; `Summarize` math; Hebrew formatting; date-parser tolerance; `formatILS` thousands separator. |
| `internal/forms` | `collector_test.go` | Form lookup, state machine `NextField` / `IsComplete`, field validation (id 9 digits, numeric), Hebrew summary. |

## Environment variables

| Var | Required | Description |
|---|---|---|
| `ANTHROPIC_API_KEY` | yes (to actually talk to Claude) | API key for the Messages API. |
| `SESSION_SECRET` | yes in prod | HMAC key for gorilla cookie store. Use a long random string. |
| `PORT` | no | Default `8080`. |

## Security notes

- The full system prompt lives in `internal/anthropic/system_prompt.go` as a Go
  `const`. It is injected server-side on every request. The frontend has no
  way to send or modify it — `/api/chat` ignores any `system` field in the body.
- Session cookie is `HttpOnly`, `SameSite=Lax`, 7-day expiry.
- `securityHeaders` middleware sets `X-Content-Type-Options: nosniff`,
  `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`.
- Request body is capped at 64 KB.

## Scope of the MVP

**In:** Claude chat with mode-aware system prompt, gorilla/sessions, RTL Hebrew UI,
data.gov.il market summaries, form field schemas, hermetic test suite.

**Out:** real authentication, DB-backed history, actual PDF form generation,
Madlan/Yad2 integrations, multi-tenant.
