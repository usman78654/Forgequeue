# Go API client

The initial versioned client lives at `github.com/usman78654/Forgequeue/client/v1`. It requires Go 1.23 and has no additional dependencies. It supports typed job submission, inspection, cancellation, requeue and cursor pages; `Do` covers other JSON v1 endpoints with application-owned response types. Streaming events and artifact transfers are not supported by this initial client.

```go
api, err := v1.New(os.Getenv("FORGEQUEUE_URL"), os.Getenv("FORGEQUEUE_API_KEY"), nil)
if err != nil { return err }
page, err := api.Jobs(ctx, v1.JobFilter{Queue: "jobs.default", Limit: 50})
if err != nil { return err }
for _, job := range page.Items { fmt.Println(job.ID, job.Status) }
// Fetch another page with Cursor: page.NextCursor when it is nonempty.
```

Supply contexts with deadlines appropriate to each operation. Requests also have a 30-second client timeout and 16-MiB response limit. HTTPS is required for production credentials. Redirects are rejected to avoid forwarding credentials. Only GET service errors 502/503/504 retry automatically, with at most two short backoffs; writes never retry. Use submission idempotency keys when intentionally repeating a job submission after an ambiguous transport failure. Inspect typed errors using `errors.As(err, &apiErr)` with `var apiErr *v1.APIError`; do not log sensitive response details indiscriminately.

`queuectl` reads the same URL/key environment variables and now includes `queues`, `workers`, `dead`, `diagnostics`, `audit`, `attempts`, `events`, `artifacts`, `requeue`, and `queue-control`. List output is a page, not an export of the entire dataset.
