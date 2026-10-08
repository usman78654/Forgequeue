package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	base := os.Getenv("FORGEQUEUE_URL")
	if base == "" {
		base = "http://localhost:18080"
	}
	if len(os.Args) < 3 {
		usage()
	}
	command, idOrFile := os.Args[1], os.Args[2]
	switch command {
	case "submit":
		request("POST", base+"/v1/jobs", read(idOrFile))
	case "workflow":
		request("POST", base+"/v1/workflows", read(idOrFile))
	case "schedule":
		request("POST", base+"/v1/schedules", read(idOrFile))
	case "get":
		request("GET", base+"/v1/jobs/"+idOrFile, nil)
	case "get-workflow":
		request("GET", base+"/v1/workflows/"+idOrFile, nil)
	case "cancel":
		request("POST", base+"/v1/jobs/"+idOrFile+"/cancel", nil)
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
	resp, e := http.DefaultClient.Do(req)
	check(e)
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Printf("HTTP %d\n%s", resp.StatusCode, out)
	if resp.StatusCode >= 400 {
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
  cancel JOB_ID          cancel a job`))
	os.Exit(2)
}
func check(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
