# ForgeQueue console

Open `/app/` on your API host and connect with an API key. The console exchanges the key for an HttpOnly session. Keys are never saved in browser storage. Production hosts must use HTTPS with secure session cookies.

Navigation uses hash URLs such as `/app/#/jobs` and `/app/#/workers`; page refresh and browser back/forward preserve the selected page. The console waits for session restoration before loading protected data.

## Roles and operations

Viewers can inspect resources. Operators can submit, cancel and requeue their jobs, manage workflows and schedules, and control queues. Admins additionally manage API keys. Consequential mutations require a confirmation dialog and disable repeated submissions while pending. Backend authorization remains authoritative.

Jobs and workflows use cursor pages. Job status, queue and type filters restrict requests; counts summarize the visible page rather than all jobs in the database. Workflow step details load only when requested. Background reads cancel when the page changes, and reconnecting or returning to the tab refreshes list data.

Inspect a job to view durable events, live progress and logs, worker attempt history and errors, and artifact metadata. Live streams resume using event IDs and retain the newest 1,000 events. Artifact uploads currently accept files up to 50 MiB and show a transfer pending state; the server's configured limit is authoritative. Downloads use short-lived object-storage URLs. Operators and administrators can delete artifacts after confirmation. Percentage transfer progress remains planned.

## Keyboard access

Use Tab to reach navigation, fields and actions, and Enter or Space to activate buttons. Dialogs trap focus, Escape closes dismissible dialogs, and focus returns to the launching control. Pending consequential actions cannot be dismissed until the request completes. Tables scroll within their panels on small screens.

## Troubleshooting

- An expired session returns to the connection form. Reconnect with a valid key.
- An offline warning means requests may not have reached the server. Inspect the resource before repeating a consequential action.
- Temporary service errors are retried for reads only. Write requests are never automatically retried.
- Live activity shows Reconnecting when the stream drops. The browser resumes with its last event ID.
- Artifact service errors indicate storage configuration or connectivity problems. Check API readiness and MinIO/S3 settings.
- API key revocation prevents new requests using that key; browser sessions have an independent lifecycle.

## Current limits

Administration includes principal/key creation, quota changes and search across the newest 200 audit records. New plaintext keys appear only once and are cleared when their dialog closes. The builder supports payload editing, dependency validation, deletion and JSON import/export. Unsaved changes warn before closing the browser or navigating away; export JSON to retain a copy. The toolbar offers persistent light/dark themes and optional deployment-configured observability links.

Resource-specific deep links, visual regression baselines and full audit pagination remain under development. Tests using mocked browser responses verify UI behavior; real service integration tests are a separate gate.
