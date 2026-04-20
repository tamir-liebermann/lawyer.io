# CLAUDE.md

## Project: Israeli Real Estate Law Office AI — MVP

### Tech Stack
- Backend: Go 1.22+
- AI: Anthropic Claude API (claude-sonnet-4-20250514)
- Frontend: Vanilla HTML/CSS/JS with RTL Hebrew support
- Data: data.gov.il public API

### Running the project
```
go run ./cmd/server/main.go
# Serves on :8080
```

### Environment Variables
```
ANTHROPIC_API_KEY=...
SESSION_SECRET=...
PORT=8080
```

### Code conventions
- All comments in English
- All user-facing strings in Hebrew
- RTL: set `dir="rtl"` on body, use `margin-inline-start` not `margin-left`
- Error messages: log in English, display in Hebrew

### Key files
- `internal/anthropic/client.go` — Claude API calls, system prompt injection
- `internal/chat/handler.go` — main chat endpoint
- `internal/realestatedata/fetcher.go` — data.gov.il client
- `web/static/app.js` — frontend chat logic

### System prompt location
The full system prompt is in `internal/anthropic/system_prompt.go` as a const string.
Inject it on every API call. Do NOT let the frontend send or modify the system prompt.

### User type detection
- URL param: `?mode=lawyer` sets lawyer mode for the session
- Default: client mode
- Store in gorilla/sessions cookie

### data.gov.il API
Base URL: https://data.gov.il/api/3/action/datastore_search
Real estate dataset resource ID: 5c78e9fa-c2e2-4771-93ff-7f400a12f7ba
Filter by city name in Hebrew. Return last 6 months of transactions.
