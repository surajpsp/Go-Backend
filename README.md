# Product API — Go + Gin

Backend for the Kotlin Multiplatform assignment (Part A). A small REST API for
managing products, backed by SQLite.

## Requirements

- Go 1.25 or newer

No other setup: the SQLite driver is [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite),
a pure-Go implementation, so there is no CGO toolchain or system SQLite to install.

## Running

```bash
go run . -seed
```

The server listens on `http://localhost:8080` and creates `products.db` in the
working directory on first run. `-seed` inserts 120 demo products; it skips SKUs
that already exist, so it is safe to repeat.

Check it is up:

```bash
curl http://localhost:8080/health
```

Stop with `Ctrl-C` — the server drains in-flight requests before exiting.

### Options

Every flag has an environment-variable equivalent; the flag wins when both are set.

| Flag | Env | Default | Description |
| --- | --- | --- | --- |
| `-addr` | `ADDR` | `:8080` | host:port to listen on |
| `-db` | `DB_PATH` | `products.db` | SQLite database file |
| `-log-dir` | `LOG_DIR` | `logs` | directory for daily JSON log files |
| `-seed` | — | off | insert demo products, then start |
| `-seed-only` | — | off | insert demo products and exit |

```bash
go run . -addr :9000 -db /tmp/demo.db
```

### Connecting from a device

`localhost` on a phone or emulator is the device itself, not your machine:

- **Android emulator** — `http://10.0.2.2:8080`
- **iOS simulator** — `http://localhost:8080` works as-is
- **Physical device** — use your machine's LAN IP (`ipconfig getifaddr en0` on
  macOS) and make sure both are on the same network.

## Tests

```bash
go test ./...
```

Covers the store (CRUD, duplicate SKUs, stock arithmetic, concurrent
adjustments, search escaping) and the HTTP layer end to end through the real
router — validation rules, status codes, and the error envelope.

## API

Base URL `http://localhost:8080`.

### Response envelope

Every response — success or failure — has the same shape, so a client has one
parse path.

```jsonc
// success
{"status":"success","code":200,"message":"...","data": { } }

// failure
{"status":"error","code":404,"message":"product not found",
 "data":null,"error":"not_found","requestId":"a1b2c3d4"}
```

`error` is a stable machine-readable slug — branch on it, not on the message.
`requestId` also comes back in the `X-Request-ID` header and appears in the
server log, so a reported id maps to a log line.

### `GET /products`

Returns all products.

Optional query parameters:

| Param | Description |
| --- | --- |
| `search` | case-insensitive substring match on name or SKU |
| `page` | 1-based page number (default 1) |
| `limit` | page size, capped at 100 |

Pagination is opt-in: without `page` or `limit` the full set is returned. The
`pagination` block is always present.

```jsonc
{"status":"success","code":200,"message":"products fetched successfully",
 "data":[{"id":1,"name":"Wireless Keyboard","sku":"KB-001","price":35.50,"stockQuantity":10}],
 "pagination":{"page":1,"limit":1,"total":1,"totalPages":1}}
```

### `GET /products/{id}`

Returns one product. `404 not_found` if it does not exist, `400 invalid_id` if
the id is not a number.

### `POST /products`

```json
{"name":"Wireless Keyboard","sku":"KB-001","price":35.50,"stockQuantity":10}
```

Validation, all reported as `400 validation_failed` naming the JSON field:

- `name` — required, not blank (leading/trailing whitespace is trimmed)
- `sku` — required, not blank, unique
- `price` — greater than 0
- `stockQuantity` — 0 or more; defaults to 0 if omitted

`201` on success. A SKU that already exists returns `409 duplicate_sku`.

### `POST /products/{id}/stock`

```json
{"quantity": -2}
```

A positive quantity adds stock, a negative one removes it, and `0` is an
accepted no-op. Returns the full updated product, so the client can display the
quantity the server actually stored.

The resulting stock may not go below zero: an adjustment that would take it
negative is rejected with `409 insufficient_stock` and leaves the stock
unchanged. The read-check-write runs in a transaction, so concurrent
adjustments cannot oversell.

### `GET /health`

Liveness probe. Useful for confirming a phone or emulator can reach the backend
before suspecting the app.

### Error codes

| Slug | Status | Meaning |
| --- | --- | --- |
| `validation_failed` | 400 | body or query parameters failed validation |
| `invalid_id` | 400 | path id is not a number |
| `not_found` | 404 | no product with that id |
| `route_not_found` | 404 | no such endpoint |
| `method_not_allowed` | 405 | wrong method for that path |
| `duplicate_sku` | 409 | SKU is already taken |
| `insufficient_stock` | 409 | adjustment would make stock negative |
| `internal_error` | 500 | unexpected failure; details are in the log only |

## Layout

```
main.go                  flags, startup, graceful shutdown
internal/router/         middleware chain + route table
internal/handlers/       HTTP layer: bind, validate, map errors to responses
internal/models/         product and request types
internal/store/          Store interface + SQLite implementation
internal/response/       the JSON envelope
internal/middleware/     request id, request logging, recovery, body limit
internal/logger/         slog setup: console text + daily JSON file
internal/seed/           demo dataset
```

Handlers depend on the `store.Store` interface rather than the SQLite type, so
the storage engine can be swapped without touching the HTTP layer.

## Logging

One structured line per request, written to the console as text and to
`logs/app-YYYY-MM-DD.log` as JSON, rotating daily. 5xx logs at error level, 4xx
at warn. Underlying causes — driver and SQL errors — are recorded server-side
only; clients get a generic message.

## Notes and trade-offs

- **SQLite over in-memory.** Data survives a restart, which makes the app easier
  to work against, and the assignment allows either.
- **Price as `REAL`.** Fine for a demo; a production system should store minor
  units as an integer to avoid float rounding.
- **No auth, no rate limiting, no CORS.** Out of scope for a locally-run
  assignment backend. CORS is not needed by native Android/iOS clients.
