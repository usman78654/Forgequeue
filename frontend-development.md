# ForgeQueue Frontend Development Plan

This document is the source of truth for frontend work. Status changes must be made when implementation begins and only moved to `COMPLETE` after the listed verification passes.

Status legend: `PLANNED`, `IN PROGRESS`, `COMPLETE`, `NEEDS ENVIRONMENT`, `BLOCKED`.

## Phase 0 — Production-ready foundation

| # | Work item | Status | Completion criteria |
|---|---|---|---|
| F-01 | Modular application architecture | COMPLETE | API client, application shell, shared components, and feature pages are separated; no monolithic application file remains; production build passes |
| F-02 | Secure API-key experience | COMPLETE | No key is prefilled or persisted to `localStorage`; unauthenticated UI does not issue protected requests; connect/disconnect states are explicit |
| F-03 | Responsive application shell | COMPLETE | All navigation remains reachable at 390 px; actions do not clip; tables scroll within their panels; DAG layout adapts without page-level overflow |
| F-04 | Accessible interaction primitives | COMPLETE | Navigation exposes current state; dialogs have names, focus management, Escape close, and focus restoration; notifications use appropriate live regions; visible focus styles exist |
| F-05 | Request and form UX states | COMPLETE | Pages expose loading, empty, error, success, and pending states; invalid JSON is handled without a render crash; duplicate submissions are prevented |
| F-06 | Deterministic embedded bundle | COMPLETE | `npm run build` writes the reviewed production bundle into the Go embed directory, removes stale hashed assets, and CI rejects an out-of-date bundle |

## Phase 1 — Engineering quality

| # | Work item | Status | Completion criteria |
|---|---|---|---|
| F-07 | TypeScript migration | COMPLETE | Components, API contracts, forms, and status values are typed with no unchecked `any` at network boundaries |
| F-08 | Linting and formatting | COMPLETE | ESLint and Prettier run locally and in CI with React hooks and accessibility rules enabled |
| F-09 | Component and hook tests | COMPLETE | Vitest and Testing Library cover API errors, authentication state, job submission validation, dialogs, navigation, and empty/loading states |
| F-10 | Automated accessibility checks | COMPLETE | axe tests pass for every primary page and dialog; keyboard-only smoke tests cover navigation and submission |
| F-11 | End-to-end tests | COMPLETE | Playwright covers connect, submit, inspect, cancel, retry, workflow creation, schedules, responsive navigation, and API failures |
| F-12 | Visual regression tests | PLANNED | Desktop and mobile baselines cover primary pages, dialogs, errors, empty states, and dense data tables |

## Phase 2 — Operations feature parity

| # | Work item | Status | Completion criteria |
|---|---|---|---|
| F-13 | Live job activity | IN PROGRESS | Resumable SSE drives live progress, structured logs, connection state, and retry timelines without polling loops |
| F-14 | Artifact management | IN PROGRESS | Authorized users can upload, list, download, and delete artifacts with progress, limits, expiry, and clear failure states |
| F-15 | Queue operations | IN PROGRESS | Operators can inspect queue depth, pause/resume queues, change concurrency caps, and see the effect on routing |
| F-16 | Worker operations | IN PROGRESS | Worker capabilities, queues, draining/offline state, heartbeat age, and active work are visible and filterable |
| F-17 | Dead-letter operations | IN PROGRESS | Dead jobs support inspection, selection, safe bulk requeue, and attempt/error comparison |
| F-18 | Audit and tenant administration | IN PROGRESS | Admin pages cover principals, one-time key creation, revocation, quotas, and searchable audit history without redisplaying secrets |
| F-19 | Observability deep links | IN PROGRESS | Job, workflow, worker, and queue views link to relevant metrics, traces, and runbooks using configured external URLs |

## Phase 3 — Product polish

| # | Work item | Status | Completion criteria |
|---|---|---|---|
| F-20 | Reusable design system | IN PROGRESS | Tokens and shared primitives cover color, typography, spacing, buttons, fields, badges, cards, tables, and overlays in light/dark themes |
| F-21 | Advanced DAG editor | IN PROGRESS | Keyboard-accessible node editing, validation, deletion, layout, step configuration, import/export, and unsaved-change protection are implemented |
| F-22 | URL routing and deep links | IN PROGRESS | Primary pages and resource detail views have stable URLs, refresh safely, and support browser back/forward navigation |
| F-23 | Large-data behavior | IN PROGRESS | Cursor pagination, filters, debouncing, virtualization where justified, cancellation, and stale-request protection handle production-sized datasets |
| F-24 | User-facing documentation | IN PROGRESS | Console setup, roles, workflows, troubleshooting, keyboard behavior, screenshots, and security expectations are documented |

## Phase 4 — Security, resilience, and scale

| # | Work item | Status | Completion criteria |
|---|---|---|---|
| F-25 | HttpOnly web sessions | COMPLETE | API keys are exchanged once for hashed, expiring, revocable server-side sessions using Secure/HttpOnly/SameSite cookies; unsafe requests require CSRF tokens; CLI Bearer authentication remains supported |
| F-26 | Query and cache architecture | COMPLETE | Requests support cancellation, deduplication, invalidation, bounded retry, background refresh, and stale-response protection without N+1 page loading |
| F-27 | Error boundaries and degraded mode | COMPLETE | Render failures, expired sessions, offline state, and degraded broker/storage dependencies show recoverable screens with safe diagnostic context |
| F-28 | Destructive-action safety | COMPLETE | Cancellation, key revocation, bulk requeue, queue pausing, and other consequential operations use role-aware confirmation and pending-state patterns |
| F-29 | Bundle and runtime performance | COMPLETE | Route-level lazy loading excludes the DAG editor from initial load; bundle budgets, memoization, and justified virtualization are enforced in CI |

## Verification log

- `2026-10-11` - Current enhancement batch: 40 component/API/hook tests, 20 desktop/mobile Playwright workflows, TypeScript, lint, formatting, production build and existing bundle budgets pass. New coverage includes stale requests, resumable activity, nested dialog dismissal, cyclic DAG imports, unsaved navigation guards, one-time secrets and quota confirmations. Broader frontend acceptance gaps remain explicitly IN PROGRESS.

- `2026-10-11` - Completed F-01 by removing the unused monolithic entry and legacy API client. The application now builds exclusively from the modular shell, components, feature pages, and typed API boundary.
- `2026-10-11` - Completed F-08. TypeScript checking, ESLint with React Hooks and accessibility rules, Prettier, Vitest, Testing Library, axe, and Playwright are wired into local scripts and CI. F-07, F-09, F-10, and F-11 remain in progress because their full migration and coverage criteria are broader than the initial gate infrastructure.
- `2026-10-11` - Completed F-25. The console exchanges API keys for hashed, expiring, revocable HttpOnly/SameSite sessions; unsafe cookie-authenticated requests require a rotating CSRF token; production secure cookies default on; CLI Bearer authentication remains available.
- `2026-10-11` - Added F-26 through F-29 as the remaining planned improvements: query/cache architecture, degraded-mode resilience, destructive-action safety, and bundle/runtime performance.
- `2026-10-11` - Verification passed: frontend quality suite (6 tests), production build, Playwright desktop/mobile (2 tests), migration upgrade, full Go unit/integration suite, Go vet, and OpenAPI validation.
- `2026-10-11` - Completed F-07 and F-09 through F-11. Production modules and API contracts are typed; 24 component/API/accessibility tests and 14 desktop/mobile Playwright workflows cover authentication, page states, keyboard use, dialogs, submissions, operational actions, and failures.
- `2026-10-11` - Completed F-27. A page-level error boundary isolates render failures, 401 responses return users to a recoverable connection screen, 502/503/504 responses expose dismissible degraded-service state, and browser connectivity changes expose an offline warning.
- `2026-10-11` - Completed F-28. Role-aware confirmation and pending-state protection now cover job and workflow cancellation, individual and bulk dead-job retry, schedule enable/disable, queue pause/resume and concurrency changes, and admin-only API-key revocation. Viewers receive read-only controls, and the API exposes key metadata without secret values so admins never need to enter opaque identifiers.
- `2026-10-11` - Completed F-29. The DAG editor and React Flow CSS are lazy-loaded as separate chunks. CI enforces 250 KiB initial JavaScript, 16 KiB initial CSS, and 200 KiB lazy-editor budgets; after the operational safety pages, the current raw sizes are 243.6 KiB, 9.2 KiB, and 176.3 KiB respectively.
- `2026-10-11` - Frontend verification passed: TypeScript, ESLint, Prettier, 31 Vitest component/API/accessibility tests, production build, bundle budgets, and all 20 Playwright desktop/mobile workflows. The full Go suite and OpenAPI validation also pass.

- `2026-10-08` — Plan created after source review plus rendered desktop and 390 px mobile inspection. Phase 0 implementation started.
- `2026-10-08` — Completed memory-only authentication, responsive shell and data panels, accessible navigation/dialog/notification primitives, controlled request states, and deterministic Go-embedded builds. Modularization remains in progress until the unused legacy entry is removed.

- `2026-10-11` - Completed F-26: abortable reads, stale-response protection, GET deduplication, opt-in short cache, mutation invalidation, bounded retries and background refresh. Workflow lists no longer perform N+1 requests. F-13 through F-24 are IN PROGRESS; implementation-plan.md records their remaining acceptance criteria.
