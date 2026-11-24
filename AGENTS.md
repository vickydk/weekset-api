# Agent instructions (Weekset API)

This file is the **canonical** guide for AI coding assistants working in
this repository (Cursor, Claude Code, Copilot, etc.).
**CLAUDE.md** and **Cursor rules** only point here — keep substantive
updates in this file.

These working rules come from STRAOUR. The product does not. Do not copy
STRAOUR domains (rides, routes, motorcycles, Firebase Auth) into this repo.

## Quick orientation

- **App:** Weekset API — a Go HTTP API for Weekset, one fitness center.
  Sibling product: Weekset mobile. Product scope:
  [Weekset on Notion](https://app.notion.com/p/c9f885f46cd582c0ae6f811d74793e03).
  Phase 1 modules: identity, center membership, schedule, session logs,
  exercise plans, and notifications.
- **Docs:** [README.md](./README.md) for product and stack, when present.
  How to write Go: [docs/code-guidelines.md](./docs/code-guidelines.md)
  (hexagonal architecture, logging, errors, mockery, ozzo-validation).
  See [Where to look](#where-to-look).

## Before changing code

1. Skim [README.md](./README.md) and
   [docs/code-guidelines.md](./docs/code-guidelines.md).
2. **New features follow hexagonal architecture:** `adapter/http` →
   `usecase` → `port` → `domain`, with Postgres only in
   `internal/adapter/postgres`. Wire in `cmd/api`. Never put SQL in a
   handler.
3. **Do not log passwords, tokens, workout notes, or PII** (email, phone,
   names, sets, reps, load). Log UUIDs only. See the logging section of
   code-guidelines.
4. Every query that reads tenant data is limited by `fitness_center_id`
   and by role. A trainer sees only assigned clients. Center staff see
   the roster and the timetable, not workout notes.
5. Prefer implementing over proposing, and run real commands (build,
   vet, tests) instead of only suggesting them.

## Tooling

- Go version: whatever `go.mod` specifies once it exists (target a
  current stable Go 1.23+).
- Run locally (once scaffolded): `APP_ENV=development go run ./cmd/api`
  (port from `APP_PORT`, default `8080`).
- Env: `APP_ENV`, `APP_PORT`, `DATABASE_URL`. Copy from `.env.example`
  when that file exists. Never commit `.env`, tokens, or other
  credentials. Those paths are gitignored; do not `git add -f` them.
- Dependencies: modules, not vendor. After a new import: `go mod tidy`.
- Migrations: SQL files under `migrations/` (golang-migrate). IDs are
  UUID; `created_at` / `updated_at` are UTC `TIMESTAMPTZ`.
- Format: `gofmt` (or `go fmt ./...`). Also `go vet ./...` on anything
  you touch.
- Prefer running real commands over only suggesting them.

## Testing

- Default suite is **unit tests with mockery mocks** — no Postgres.
  This must stay green.
- Run: `go test ./...` (or `gotestsum ./...` if installed).
- A subset: `go test ./... -run TestNamePattern`.
- Coverage: `go test ./... -cover`. New/changed use-case code should
  have unit tests.
- Register interfaces in `.mockery.yml` and run `mockery` from the repo
  root after adding/changing a port. Pattern:
  [docs/code-guidelines.md § Unit testing with mockery](./docs/code-guidelines.md#unit-testing-with-mockery).
- **New tests default to the unit set.** Only add `//go:build integration`
  when the test cannot work without real infrastructure.
- Do not write new tests that require a running server on localhost.

## Swagger docs

**Every new or changed HTTP API must ship Swagger in the same change.**
Do not add a route without swaggo comments, DTO `example` tags, and
regenerated `docs/docs.go` / `docs/swagger.json` / `docs/swagger.yaml`.
A PR that changes an endpoint but not those files is incomplete.

Write the spec for the **Weekset Flutter app** (`weekset` repo), not
only for backend reviewers:

- `@Summary` + `@Description` must say *when* to call the endpoint,
  which header to send (`Authorization: Bearer <access token>`), what
  to persist, and how to branch on status.
- Document every real status the handler returns (`@Success` /
  `@Failure`) using `apierror.ErrorResponse` for errors.
- Put `example:"..."` on JSON DTO fields. The client does not choose
  its role or its center. Those come from the membership.
- Keep comments in sync with ozzo-validation and `platform/apierror`.

Regenerate after adding, removing, or changing an endpoint:

```bash
make swagger
# or: swag init --parseDependency --parseInternal -g cmd/api/main.go -o docs
```

Install if missing: `go install github.com/swaggo/swag/cmd/swag@v1.16.4`.
UI is served at `/docs/index.html` (OpenAPI JSON at `/docs/doc.json`).

## Before opening a PR

Nothing in CI may catch these yet — run them yourself:

1. `go build ./...`
2. `go vet ./...`
3. `gofmt` / `go fmt ./...`
4. Tests relevant to the change — see [Testing](#testing).
5. If you touched handler swaggo comments or added/removed an endpoint,
   regenerate swagger (see [Swagger docs](#swagger-docs)).
6. If you touched `migrations/`, confirm up and down against local
   Postgres.

## Opening the PR

- Feature branches off `main`. Do not push or open a PR unless the
  user asks.
- Conventional-commit-style messages: `type(scope): description`
  (`feat`, `fix`, `docs`, `test`, `chore`). Check `git log --oneline`
  once history exists.
- Write the PR description for a reviewer (summary + test plan). Do
  not force-push to `main`.

## Where to look

| Topic | Location |
|---|---|
| Product scope | [Weekset on Notion](https://app.notion.com/p/c9f885f46cd582c0ae6f811d74793e03) |
| Product / stack, once written | [README.md](./README.md) |
| Hexagonal layout, logging, errors, tests, validation | [docs/code-guidelines.md](./docs/code-guidelines.md) |
| Cursor-specific snippets | [.cursor/rules/](./.cursor/rules/) |
| Env vars | `.env.example` (when present) |

## Conflicts

If this file and [docs/code-guidelines.md](./docs/code-guidelines.md)
disagree on something narrow and technical, prefer code-guidelines.
For repo-wide process (PRs, what to run before opening a PR), prefer
this file. If **CLAUDE.md** or a Cursor rule says something
substantively different from this file, treat that as a stale pointer
— follow this file and flag the discrepancy.
