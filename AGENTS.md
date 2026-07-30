# Repository Guidelines

## Project Structure & Module Organization

BTaskAssistant combines a Go/Wails backend with a React/TypeScript UI. Desktop entry points are `main.go` and `app.go`. Packages under `internal/` cover workflow transitions, SQLite storage, Plane integration, AI adapters, and credentials. UI code lives in `frontend/src`; tests are colocated as `*.test.ts` or `*_test.go`. Architecture decisions, release notes, and scope documents belong in `docs/`. Wails settings are in `wails.json`.

## Build, Test, and Development Commands

Use Node.js 20+ (CI uses 22), pnpm 11.7, Go 1.25, and Wails v2.

```bash
cd frontend && pnpm install --frozen-lockfile
pnpm dev                 # Start the browser-based Vite preview
pnpm typecheck           # Run strict TypeScript checks
pnpm test                # Run Vitest once
pnpm build               # Type-check and create the frontend bundle
```

From the repository root:

```bash
go test ./internal/...   # Match the backend CI suite
go test ./...            # Include root application tests
wails doctor && wails dev
wails build              # Produce the desktop binary
```

## Coding Style & Naming Conventions

Format changed Go files with `gofmt`; use lowercase package names, exported `PascalCase` identifiers, and concise error context. TypeScript uses strict mode, two-space indentation, double quotes, semicolons, and trailing commas. Name React components and files in `PascalCase` (`TaskDetail.tsx`), variables in `camelCase`, and constants in `UPPER_SNAKE_CASE`. Keep domain and store modules lowercase (`domain/workflow.ts`).

## Testing Guidelines

Frontend tests use Vitest with `jsdom` and live under `frontend/src/**/*.test.ts`. Go tests use the standard `testing` package and `TestXxx` names. There is no enforced coverage threshold; add focused regression tests for changed behavior, especially workflow gates, persistence, Plane deduplication, and credential boundaries. Run both frontend and Go suites before opening a PR.

## Commit & Pull Request Guidelines

History uses Conventional Commit-style prefixes such as `feat:`, `fix:`, `build:`, and `docs:`. Keep summaries short and commits focused. PRs should explain the problem and solution, link issues, list validation commands, and include screenshots for UI changes. Call out changes to SQLite data, credentials, integrations, or workflow transitions.

## Security & Workflow Invariants

Never write Plane PATs to SQLite, frontend state, logs, fixtures, or commits; use the OS credential store. Token-bearing remote requests must use HTTPS or loopback addresses. Do not bypass the one-stage transition rule or any explicit human confirmation gate.
