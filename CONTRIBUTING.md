# Contributing

Use a feature branch and describe the user-visible change, relevant reliability semantics and verification in the pull request. Do not commit credentials, dependency directories or test output.

Go changes should pass formatting, vet and race tests. Integration tests require TEST_DATABASE_URL and TEST_RABBIT_URL with migrated disposable services. Artifact tests additionally require object storage. Do not treat skipped integration tests as successful live verification.

Frontend changes must pass `npm run quality`, `npm run build`, `npm run bundle:check` and relevant Playwright workflows from the frontend directory. Commit the production assets emitted directly into internal/httpapi/web.

Record feature statuses and actual verification in implementation-plan.md and the relevant roadmap. Preserve migration checksums. Add forward migrations for schema changes and document compatibility.

Use semantic versioning for releases. Create a release tag only after CI and service smoke checks pass. The tag workflow publishes container images and a GitHub Release. Licensing and copyright identity require the repository owner's decision.
