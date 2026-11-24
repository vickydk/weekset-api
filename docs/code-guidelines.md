# Code Guidelines

Practical rules for writing Go in this repo: hexagonal layout, logging,
API errors, ozzo-validation, and mockery tests. This is the source of
truth for *how* we write code. [AGENTS.md](../AGENTS.md) covers *how
assistants should work* (tooling, tests, PRs). Product scope lives on
the [Weekset Notion page](https://app.notion.com/p/c9f885f46cd582c0ae6f811d74793e03).

Weekset is the shared week for one fitness center. The API is consumed
by the mobile app. Passwords, tokens, and workout notes are secrets.
The center can see the timetable and the roster, not the note.

## Project-specific guidelines

- **Hexagonal architecture is required** for every feature. Handlers do
  not talk to Postgres. Use cases do not import the HTTP or SQL
  packages. See [Code structure](#code-structure).
- **Composition root is `cmd/api`.** Only that package (and tests)
  constructs adapters and injects them into use cases.
- **API versioning is `/v1/...`.** Do not invent `/v2` until we need a
  breaking change.
- **Identity is email and password.** Hash passwords with Argon2id.
  Issue a short-lived access token and a rotating refresh token. Rate
  limit login. The client does not choose Trainer or Center admin on
  sign-up. The center grants the role on the membership.
- **`APP_ENV` gates log level and similar behavior.** `development`
  locally (including unit tests unless a test sets otherwise);
  `staging` / `production` in deployed environments. Debug logs exist
  only in `development`.
- **IDs are UUID. Timestamps are `TIMESTAMPTZ` stored and returned in
  UTC.** Tenant rows carry `fitness_center_id`.
- **Booking rules live in the schedule use case.** A trainer cannot
  hold two sessions at the same time. A recurring series generates
  concrete bookings. Skipping one date leaves the series in place.
  Cancelling a confirmed session frees the slot. Schedule changes and
  session-log edits write an audit entry.
- **Do not vendor dependencies** unless we later decide to. After adding
  an import: `go mod tidy`. Do not commit `vendor/`.
- **Match the file you are editing.** This repo will grow; keep a package
  internally consistent even if a neighbor is slightly different.

## Best practices

- **Don't add abstractions ahead of need.** Three similar call sites is
  fine; a shared helper used once is not.
- **Business logic lives in `internal/usecase`, not in handlers or
  adapters.** A handler is decode → validate → call use case → respond.
  A repository method is a query/write, not a decision.
- **Prefer additive, backward-compatible migrations.** Expand-contract
  for renames/drops so a rolling deploy cannot break the previous
  release.
- **Small interfaces, defined where they are consumed** (ports live in
  `internal/port`, implemented in `internal/adapter/...`).
- **Context is the first argument** on use case, port, and adapter
  methods that do I/O. Never store `context.Context` on a struct.
- **No global DB/session pool outside the composition root.** Pass
  dependencies in constructors.

## Code structure

Hexagonal (ports and adapters). Dependencies point inward:

```text
cmd/api  →  adapter  →  usecase  →  port  →  domain
                 ↘________________↗
```

```text
weekset-api/
├── cmd/api/                    # main, wire adapters → use cases → HTTP
├── internal/
│   ├── domain/                 # entities, value objects, domain errors
│   │   ├── user/
│   │   ├── membership/
│   │   ├── schedule/
│   │   ├── sessionlog/
│   │   ├── exercise/
│   │   └── soloplan/
│   ├── port/                   # inbound + outbound interfaces
│   ├── usecase/                # application services (one package per feature)
│   ├── adapter/
│   │   ├── http/               # driving: router, middleware, handlers, DTOs
│   │   │   ├── middleware/
│   │   │   └── auth/
│   │   └── postgres/           # driven: repositories
│   └── platform/
│       ├── logger/             # structured logger, stdout
│       └── apierror/           # HTTP error helpers
├── migrations/
└── docs/                       # code-guidelines.md + generated swagger
```

Layer rules:

1. **`domain/`** — pure Go. No HTTP, SQL, or logger imports.
   Entities and sentinel errors (`ErrNotFound`, `ErrUnauthorized`).
2. **`port/`** — interfaces only. Outbound: repositories, `Clock`,
   password hasher, token issuer. Inbound: the service HTTP calls.
3. **`usecase/<feature>/`** — implements inbound ports. Orchestrates
   domain + outbound ports. No `net/http` or `pgx`.
4. **`adapter/http/`** — Echo handlers and middleware.
   Decode JSON, run ozzo-validation, map domain errors to HTTP, write
   JSON. No SQL.
5. **`adapter/postgres/`** — implement outbound ports.
6. **`platform/logger`** — structured logger. **`platform/apierror`** —
   the only place that writes `{"error": ...}` JSON.

**Forbidden (do not introduce):**

- `*sql.DB` / driver types on `context.Context`
- GORM/`pgx` imports in `usecase/` or `adapter/http/`
- HTTP types in `usecase/` or `domain/`
- Trusting a role, a center id, or another person's id from the request
  body when the membership already decides access

## Logging: what to log, what not to

Use a structured logger in `internal/platform/logger`. Keep a `Logger`
interface, `Get()`, keyed `Field`, a zap backend, and request-scoped
context fields. Log to stdout (container log stream).

```go
logger.Get().Info(ctx, "booking confirmed",
    logger.Field{Key: "booking_id", Val: id},
    logger.Field{Key: "trainer_id", Val: trainerID},
)
```

- **Always pass `ctx`.** Request ID, user ID, method, and path should
  be attached in HTTP middleware so every line is correlatable. That
  only works if every call forwards the request context.
- **Pass extra data as keyed `logger.Field{Key: "...", Val: ...}`**
  (or `logger.ToField(...)`). Never positional struct literals — a
  field reorder would silently scramble log data (`go vet` composites).
- **Log identifiers freely:** user UUID, center id, booking id, HTTP
  status, route, duration.
- **Log level:** `Debug` only when `APP_ENV=development`. Use `Info`
  or higher if staging/production must see it.
- **HTTP middleware** should log method, path, status, duration,
  request ID, user ID. When a JSON body exists, also log
  `request_payload` / `response_payload` after redacting tokens,
  passwords, and workout content. Skip `/health`, `/ready`, and `/docs`.

**Do not log:**

- **Secrets and credentials** — passwords, access tokens, refresh
  tokens, `Authorization` headers, API keys.
- **PII beyond an identifier** — email, phone, full name.
- **Workout content** — exercises, sets, reps, load, the session note,
  and solo-plan content. Log `booking_id` or `session_log_id` instead.
- **Anything you need in production via `logger.Debug`.** It is dropped
  outside development.

## Error handling and what to return from the API

Helpers live in `internal/platform/apierror`. JSON shape for failures:

```json
{"error": {"code": "validation_error", "message": "email cannot be blank"}}
```

Success responses are the resource body (no `{ "data": ... }` wrapper
unless a list needs pagination later).

| Situation | HTTP | `code` | `message` |
|---|---|---|---|
| ozzo-validation failure | 400 | `validation_error` | ozzo `err.Error()` (safe) |
| malformed JSON | 400 | `invalid_request` | `invalid request body` |
| missing/invalid token, or bad password | 401 | `unauthorized` | `unauthorized` |
| authenticated but not allowed | 403 | `forbidden` | `forbidden` |
| missing row | 404 | `not_found` | `not found` |
| duplicate email, or a slot that is taken | 409 | `conflict` | specific, e.g. `email already in use` |
| unexpected (DB, panic recovered) | 500 | `internal_error` | `unexpected error` |

Rules:

- **Never send raw internal errors to the client** — no `pq:` messages,
  connection strings, or stack traces.
- **Use case returns sentinel / wrapped domain errors**
  (`fmt.Errorf("confirm booking: %w", domain.ErrConflict)`). The HTTP
  adapter maps with `errors.Is` / `errors.As` to status + code.
- **Log the real error at the adapter edge** (and report to Sentry
  later) when mapping to 500. Do not log again in the use case *and*
  the handler for the same failure.
- **Client-input messages should be specific** (`hour is already booked`).
  That is not a leak; it makes the API usable.
- Prefer `apierror.Write(w, status, code, message)` over ad-hoc
  `json.NewEncoder` in each handler.
- A bad password and a missing account return the same `unauthorized`
  message. Do not tell the client which one failed.

```go
// usecase
var ErrUnauthorized = errors.New("unauthorized")

// adapter/http
if errors.Is(err, domain.ErrUnauthorized) {
    apierror.Write(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
    return
}
if errors.Is(err, domain.ErrNotFound) {
    apierror.Write(w, http.StatusNotFound, "not_found", "not found")
    return
}
logger.Get().Error(ctx, "confirm booking failed", logger.Field{Key: "error", Val: err})
apierror.Write(w, http.StatusInternalServerError, "internal_error", "unexpected error")
```

## Swagger (required for every HTTP API)

The Flutter app (`weekset`) integrates from the OpenAPI spec. **Ship
Swagger in the same change as the handler** — comments, DTO examples,
and regenerated files under `docs/` (`docs.go`, `swagger.json`,
`swagger.yaml`). `make swagger` (or `swag init --parseDependency
--parseInternal -g cmd/api/main.go -o docs`).

Handler comments must be integration-ready:

- When the app should call the route (not only the HTTP method).
- Auth: `Authorization: Bearer <access token>` after login. Do not
  accept a role or a center id as a way to gain access.
- Success body fields to persist (especially UUID `id` and token expiry).
- Every status the handler actually returns, with `error.code` for
  failures (`apierror.ErrorResponse`).
- `example` tags on request/response DTO JSON fields.

UI: `/docs/index.html`. Do not invent `/v2` in the spec.

## Unit testing with mockery

Config-driven via `.mockery.yml` (testify template, mocks in
`<pkg>/mocks/`). Register every interface you mock:

```yaml
all: false
dir: '{{.InterfaceDir}}/mocks'
filename: '{{.InterfaceName}}.go'
pkgname: mocks
structname: '{{.InterfaceName}}'
template: testify
packages:
  github.com/vickydk/weekset-api/internal/port:
    interfaces:
      UserRepository: {}
      PasswordHasher: {}
      TokenIssuer: {}
```

Then from the repo root: `mockery`. Re-run after adding or changing a
port. Prefer `//go:generate mockery` on the interface file as a hint,
but `.mockery.yml` is authoritative.

**Test the use case with mocked ports** (see pattern below). HTTP
handlers can be tested with `httptest` and a mocked inbound port.
Postgres adapters need integration tests (`//go:build integration`) —
do not hit a real database from the default unit suite.

```go
type testDeps struct {
    repo *portmocks.UserRepository
    svc  *usecase.Service
}

func newDeps(t *testing.T) testDeps {
    t.Helper()
    repo := portmocks.NewUserRepository(t) // t.Cleanup(AssertExpectations)
    svc := usecase.NewService(repo)
    return testDeps{repo: repo, svc: svc}
}

func TestLogin_RejectsUnknownEmail(t *testing.T) {
    d := newDeps(t)
    d.repo.On("FindByEmail", mock.Anything, "maya@harbor.club").
        Return(nil, domain.ErrNotFound)

    _, err := d.svc.Login(context.Background(), usecase.LoginInput{
        Email:    "maya@harbor.club",
        Password: "secret",
    })

    require.ErrorIs(t, err, domain.ErrUnauthorized)
}
```

- **`mocks.NewX(t)` already registers `AssertExpectations`.** Do not
  call it yourself. An unused `.On(...)` fails the test.
- Use **`require`** for preconditions (`require.NoError` before using
  the result), **`assert`** for behavior so multiple failures show up.
- New tests default to **unit tests with mocks**. Only add
  `//go:build integration` when the test cannot work without Postgres.

## Request validation with ozzo-validation

Validate HTTP request bodies and query params with
[`go-ozzo/ozzo-validation`](https://github.com/go-ozzo/ozzo-validation),
not hand-rolled `if` chains. Put a `Validate() error` method on the
**HTTP DTO** in `adapter/http/<feature>/` (driving adapter). Call it
in the handler after decode, before the use case.

```go
type LoginRequest struct {
    Email    string `json:"email" example:"maya@harbor.club"`
    Password string `json:"password" example:"a-long-secret"`
}

func (r LoginRequest) Validate() error {
    return validation.ValidateStruct(&r,
        validation.Field(&r.Email, validation.Required, is.Email),
        validation.Field(&r.Password, validation.Required, validation.Length(8, 128)),
    )
}
```

```go
var req LoginRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    apierror.Write(w, http.StatusBadRequest, "invalid_request", "invalid request body")
    return
}
if err := req.Validate(); err != nil {
    apierror.Write(w, http.StatusBadRequest, "validation_error", err.Error())
    return
}
```

- Keep ozzo rules next to the DTO they validate, not in the use case.
- Use-case / domain still enforces invariants (unique email, slot is
  free, caller belongs to the center) that require I/O.
- ozzo messages (`cannot be blank`) are safe to return as `message`.
- Cross-field or custom checks: `validation.By(func(value interface{}) error { ... })`.
