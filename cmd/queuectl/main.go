package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	base := os.Getenv("FORGEQUEUE_URL")
	if base == "" {
		base = "http://localhost:18080"
	}
	if len(os.Args) < 2 {
		usage()
	}
	command, idOrFile := os.Args[1], ""
	if len(os.Args) > 2 {
		idOrFile = os.Args[2]
	}
	if idOrFile == "" && command != "queues" && command != "workers" && command != "dead" && command != "diagnostics" && command != "audit" {
		usage()
	}
	switch command {
	case "submit":
		request("POST", base+"/v1/jobs", read(idOrFile))
	case "workflow":
		request("POST", base+"/v1/workflows", read(idOrFile))
	case "schedule":
		request("POST", base+"/v1/schedules", read(idOrFile))
	case "get":
		request("GET", base+"/v1/jobs/"+url.PathEscape(idOrFile), nil)
	case "get-workflow":
		request("GET", base+"/v1/workflows/"+url.PathEscape(idOrFile), nil)
	case "cancel":
		request("POST", base+"/v1/jobs/"+url.PathEscape(idOrFile)+"/cancel", nil)
	case "requeue":
		request("POST", base+"/v1/jobs/"+url.PathEscape(idOrFile)+"/requeue", nil)
	case "attempts":
		request("GET", base+"/v1/jobs/"+url.PathEscape(idOrFile)+"/attempts", nil)
	case "events":
		request("GET", base+"/v1/jobs/"+url.PathEscape(idOrFile)+"/events", nil)
	case "artifacts":
		request("GET", base+"/v1/jobs/"+url.PathEscape(idOrFile)+"/artifacts", nil)
	case "queues":
		request("GET", base+"/v1/queues", nil)
	case "workers":
		request("GET", base+"/v1/workers", nil)
	case "dead":
		request("GET", base+"/v1/jobs?status=dead&limit=100", nil)
	case "audit":
		request("GET", base+"/v1/admin/audit", nil)
	case "diagnostics":
		request("GET", base+"/readyz", nil)
	case "queue-control":
		if len(os.Args) != 4 {
			usage()
		}
		request("PUT", base+"/v1/queues/"+url.PathEscape(idOrFile), read(os.Args[3]))
	default:
		usage()
	}
}
func read(path string) []byte {
	if path == "-" {
		b, e := io.ReadAll(os.Stdin)
		check(e)
		return b
	}
	b, e := os.ReadFile(path)
	check(e)
	return b
}
func request(method, url string, body []byte) {
	req, e := http.NewRequest(method, url, bytes.NewReader(body))
	check(e)
	req.Header.Set("Content-Type", "application/json")
	if key := os.Getenv("FORGEQUEUE_API_KEY"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, e := (&http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}).Do(req)
	check(e)
	defer resp.Body.Close()
	out, e := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	check(e)
	if len(out) > 16<<20 {
		check(fmt.Errorf("API response exceeds 16 MiB limit"))
	}
	fmt.Printf("HTTP %d\n%s", resp.StatusCode, out)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, strings.TrimSpace(`queuectl commands:
  submit FILE|-          submit a job
  workflow FILE|-        submit a workflow
  schedule FILE|-        create a recurring schedule
  get JOB_ID             inspect a job
  get-workflow ID        inspect a workflow
  cancel JOB_ID          cancel a job
  requeue JOB_ID         requeue a dead job
  attempts JOB_ID        inspect attempt errors
  events JOB_ID          inspect recent durable events
  artifacts JOB_ID       inspect artifact metadata
  queues                inspect queues and depths
  workers               inspect workers and capabilities
  dead                  list the newest 100 dead jobs
  audit                 inspect recent admin audit records
  diagnostics           check dependency readiness
  queue-control QUEUE FILE|-  apply queue control JSON

Set FORGEQUEUE_API_KEY for protected operations.`))
	os.Exit(2)
}
func check(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
