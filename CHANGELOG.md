# Changelog

## Unreleased

- Upgraded the Go baseline to 1.27.2 across the module, CI, Docker and report worker; refreshed vulnerable direct/transitive dependencies and pinned govulncheck v1.9.0. Verified zero source-vulnerability, Trivy HIGH/CRITICAL and npm-audit findings without disabling security gates.

- Added runnable offline report workflows and a dedicated typed-handler worker: parallel dataset reads, validated joins, real report files and a permanent-error/fail-fast demo, with unit and PostgreSQL integration tests.

- Added versioned Go client, authenticated maintenance commands, cursor list navigation, live job activity, attempt/artifact inspection, admin creation/quota tools, DAG import/export safeguards and theme selection.
- Added deployment overlays, monitoring CRDs and production security/placement defaults.
- Fixed artifact cleanup ordering so failed storage deletion preserves metadata, and fixed timed-out drain attempt closure by consuming query rows before transactional updates.
- Remaining acceptance criteria and environment limitations are tracked in implementation-plan.md.

### Earlier foundation work

- Secure HttpOnly browser sessions with CSRF protection and revocation.
- Modular TypeScript console, accessibility checks and desktop/mobile workflows.
- Typed Go handler registry and public worker runner.
- Confirmed queue, schedule, workflow, dead-letter and API-key operations.
- Cursor list navigation, cancellation and stale-response protection.
- Live job logs/progress, attempts and artifact controls.
- Expanded administration, editable/importable DAGs and optional observability links.

Release dates and version tags will be added after the repository owner's release decision.
