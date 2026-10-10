package httpapi

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/usman78654/Forgequeue/internal/artifacts"
	"github.com/usman78654/Forgequeue/internal/auth"
	"github.com/usman78654/Forgequeue/internal/broker"
	"github.com/usman78654/Forgequeue/internal/jobs"
	"github.com/usman78654/Forgequeue/internal/pagination"
	"github.com/usman78654/Forgequeue/internal/schedules"
	"github.com/usman78654/Forgequeue/internal/tracing"
	"github.com/usman78654/Forgequeue/internal/workflows"
)

var submitted = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "forgequeue_jobs_submitted_total", Help: "Submitted jobs."}, []string{"type"})

//go:embed web/*
var webAssets embed.FS

func init() { prometheus.MustRegister(submitted) }

type Server struct {
	store               jobs.Store
	workflows           workflows.Store
	schedules           schedules.Store
	db                  *pgxpool.Pool
	rabbit              *broker.Rabbit
	artifacts           *artifacts.Store
	mux                 *http.ServeMux
	sessionDuration     time.Duration
	secureSessionCookie bool
}

func New(db *pgxpool.Pool, rabbit *broker.Rabbit) *Server {
	s := &Server{store: jobs.Store{DB: db}, workflows: workflows.Store{DB: db}, schedules: schedules.Store{DB: db}, db: db, rabbit: rabbit, mux: http.NewServeMux(), sessionDuration: 8 * time.Hour}
	s.routes()
	return s
}

func (s *Server) ConfigureWebSessions(duration time.Duration, secureCookie bool) *Server {
	if duration > 0 {
		s.sessionDuration = duration
	}
	s.secureSessionCookie = secureCookie
	return s
}
func NewWithArtifacts(db *pgxpool.Pool, rabbit *broker.Rabbit, artifactStore *artifacts.Store) *Server {
	s := New(db, rabbit)
	s.artifacts = artifactStore
	return s
}
func (s *Server) Handler() http.Handler {
	return securityHeaders(tracing.HTTPMiddleware(logging(auth.Middleware{DB: s.db}.Wrap(s.mux))))
}
func (s *Server) routes() {
	webRoot, _ := fs.Sub(webAssets, "web")
	s.mux.Handle("GET /app/", http.StripPrefix("/app/", http.FileServer(http.FS(webRoot))))
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusTemporaryRedirect)
	})
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { jsonResponse(w, 200, map[string]string{"status": "ok"}) })
	s.mux.HandleFunc("GET /readyz", s.ready)
	s.mux.Handle("GET /metrics", promhttp.Handler())
	s.mux.HandleFunc("POST /v1/jobs", s.submit)
	s.mux.HandleFunc("GET /v1/jobs", s.listJobs)
	s.mux.HandleFunc("GET /v1/jobs/{id}", s.get)
	s.mux.HandleFunc("POST /v1/jobs/{id}/cancel", s.cancel)
	s.mux.HandleFunc("GET /v1/jobs/{id}/attempts", s.attempts)
	s.mux.HandleFunc("GET /v1/jobs/{id}/events", s.events)
	s.mux.HandleFunc("GET /v1/jobs/{id}/events/stream", s.streamEvents)
	s.mux.HandleFunc("POST /v1/jobs/{id}/requeue", s.requeue)
	s.mux.HandleFunc("POST /v1/jobs/{id}/artifacts", s.uploadArtifact)
	s.mux.HandleFunc("GET /v1/jobs/{id}/artifacts", s.listArtifacts)
	s.mux.HandleFunc("GET /v1/jobs/{id}/artifacts/{artifactID}/download", s.downloadArtifact)
	s.mux.HandleFunc("POST /v1/workflows", s.submitWorkflow)
	s.mux.HandleFunc("GET /v1/workflows", s.listWorkflows)
	s.mux.HandleFunc("GET /v1/workflows/{id}", s.getWorkflow)
	s.mux.HandleFunc("POST /v1/workflows/{id}/cancel", s.cancelWorkflow)
	s.mux.HandleFunc("POST /v1/schedules", s.createSchedule)
	s.mux.HandleFunc("GET /v1/schedules", s.listSchedules)
	s.mux.HandleFunc("POST /v1/schedules/{id}/enable", s.enableSchedule)
	s.mux.HandleFunc("POST /v1/schedules/{id}/disable", s.disableSchedule)
	s.mux.HandleFunc("GET /v1/workers", s.listWorkers)
	s.mux.HandleFunc("GET /v1/queues", s.listQueues)
	s.mux.HandleFunc("PUT /v1/queues/{queue}", s.updateQueue)
	s.mux.HandleFunc("PUT /v1/admin/quotas/{id}", s.updateQuota)
	s.mux.HandleFunc("POST /v1/admin/principals", s.createPrincipal)
	s.mux.HandleFunc("POST /v1/admin/principals/{id}/keys", s.createAPIKey)
	s.mux.HandleFunc("GET /v1/admin/keys", s.listAPIKeys)
	s.mux.HandleFunc("DELETE /v1/admin/keys/{id}", s.revokeAPIKey)
	s.mux.HandleFunc("GET /v1/admin/audit", s.auditLog)
	s.mux.HandleFunc("POST /v1/session", s.createWebSession)
	s.mux.HandleFunc("GET /v1/session", s.refreshWebSession)
	s.mux.HandleFunc("DELETE /v1/session", s.deleteWebSession)
}

func (s *Server) createWebSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, exists := auth.SessionID(r.Context()); exists {
		problem(w, http.StatusConflict, "session already active", "disconnect the current web session before creating another")
		return
	}
	principal, ok := auth.FromContext(r.Context())
	if !ok {
		problem(w, http.StatusUnauthorized, "authentication required", "a valid API key is required")
		return
	}
	session, token, err := auth.CreateWebSession(r.Context(), s.db, principal, s.sessionDuration)
	if err != nil {
		problem(w, http.StatusInternalServerError, "session creation failed", "the web session could not be created")
		return
	}
	auth.SetSessionCookie(w, token, session.ExpiresAt, s.secureSessionCookie)
	jsonResponse(w, http.StatusCreated, session)
}

func (s *Server) refreshWebSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	sessionID, ok := auth.SessionID(r.Context())
	if !ok {
		problem(w, http.StatusUnauthorized, "web session required", "no active web session was found")
		return
	}
	principal, _ := auth.FromContext(r.Context())
	session, err := auth.RotateSessionCSRF(r.Context(), s.db, sessionID, principal)
	if err != nil {
		problem(w, http.StatusUnauthorized, "session refresh failed", "the web session is no longer active")
		return
	}
	jsonResponse(w, http.StatusOK, session)
}

func (s *Server) deleteWebSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	sessionID, ok := auth.SessionID(r.Context())
	if !ok {
		problem(w, http.StatusUnauthorized, "web session required", "no active web session was found")
		return
	}
	if err := auth.RevokeWebSession(r.Context(), s.db, sessionID); err != nil {
		problem(w, http.StatusInternalServerError, "session revocation failed", "the web session could not be revoked")
		return
	}
	auth.ClearSessionCookie(w, s.secureSessionCookie)
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) uploadArtifact(w http.ResponseWriter, r *http.Request) {
	if s.artifacts == nil || !s.artifacts.Enabled() {
		problem(w, 503, "artifact storage unavailable", "object storage is not configured")
		return
	}
	name := r.Header.Get("X-Artifact-Name")
	if name == "" {
		name = r.URL.Query().Get("name")
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.artifacts.MaxBytes+1))
	if err != nil || int64(len(body)) > s.artifacts.MaxBytes {
		problem(w, 413, "artifact too large", "artifact exceeds configured size limit")
		return
	}
	a, err := s.artifacts.Put(r.Context(), r.PathValue("id"), name, contentType, body)
	if err != nil {
		problem(w, 400, "artifact upload failed", err.Error())
		return
	}
	jsonResponse(w, 201, a)
}
func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	if s.artifacts == nil {
		jsonResponse(w, 200, map[string]any{"items": []any{}})
		return
	}
	items, err := s.artifacts.List(r.Context(), r.PathValue("id"))
	if err != nil {
		problem(w, 500, "artifact lookup failed", err.Error())
		return
	}
	jsonResponse(w, 200, map[string]any{"items": items})
}
func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	if s.artifacts == nil {
		problem(w, 503, "artifact storage unavailable", "object storage is not configured")
		return
	}
	u, err := s.artifacts.DownloadURL(r.Context(), r.PathValue("id"), r.PathValue("artifactID"), 15*time.Minute)
	if err != nil {
		problem(w, 404, "artifact not found", "artifact does not exist or has expired")
		return
	}
	jsonResponse(w, 200, map[string]any{"url": u, "expires_in": 900})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if e := s.db.Ping(r.Context()); e != nil {
		jsonResponse(w, 503, map[string]string{"status": "not ready"})
		return
	}
	if !s.rabbit.Healthy() {
		jsonResponse(w, 503, map[string]string{"status": "not ready", "dependency": "rabbitmq"})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ready"})
}
func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	var req jobs.SubmitRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if e := d.Decode(&req); e != nil || req.Type == "" || !json.Valid(req.Payload) {
		problem(w, 400, "invalid request", "type and a valid JSON payload are required")
		return
	}
	if req.Priority < 0 || req.Priority > 9 {
		problem(w, 400, "invalid priority", "priority must be between 0 and 9")
		return
	}
	if p, ok := auth.FromContext(r.Context()); ok {
		req.OwnerID = p.ID
	}
	req.Traceparent = tracing.Traceparent(r.Context())
	j, created, e := s.store.Submit(r.Context(), req)
	if e != nil {
		if errors.Is(e, jobs.ErrQuotaExceeded) {
			problem(w, 429, "quota exceeded", e.Error())
			return
		}
		problem(w, 500, "submission failed", e.Error())
		return
	}
	if created {
		submitted.WithLabelValues(j.Type).Inc()
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	w.Header().Set("Location", "/v1/jobs/"+j.ID)
	jsonResponse(w, code, j)
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	j, e := s.store.Get(r.Context(), r.PathValue("id"))
	if errors.Is(e, jobs.ErrNotFound) {
		problem(w, 404, "not found", "job does not exist")
		return
	}
	if e != nil {
		problem(w, 500, "lookup failed", e.Error())
		return
	}
	jsonResponse(w, 200, j)
}
func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	after, e := parseOptionalTime(r.URL.Query().Get("created_after"))
	if e != nil {
		problem(w, 400, "invalid created_after", e.Error())
		return
	}
	before, e := parseOptionalTime(r.URL.Query().Get("created_before"))
	if e != nil {
		problem(w, 400, "invalid created_before", e.Error())
		return
	}
	ownerID := ""
	if p, ok := auth.FromContext(r.Context()); ok && p.Role != "admin" {
		ownerID = p.ID
	}
	page, e := s.store.List(r.Context(), jobs.ListFilter{Status: r.URL.Query().Get("status"), Queue: r.URL.Query().Get("queue"), Type: r.URL.Query().Get("type"), Cursor: r.URL.Query().Get("cursor"), Limit: limit, CreatedAfter: after, CreatedBefore: before, OwnerID: ownerID})
	if e != nil {
		if errors.Is(e, pagination.ErrInvalidCursor) {
			problem(w, 400, "invalid cursor", e.Error())
			return
		}
		problem(w, 500, "lookup failed", e.Error())
		return
	}
	jsonResponse(w, 200, page)
}
func (s *Server) attempts(w http.ResponseWriter, r *http.Request) {
	items, e := s.store.Attempts(r.Context(), r.PathValue("id"))
	if e != nil {
		problem(w, 500, "lookup failed", e.Error())
		return
	}
	jsonResponse(w, 200, items)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if _, e := s.store.Get(r.Context(), r.PathValue("id")); e != nil {
		if errors.Is(e, jobs.ErrNotFound) {
			problem(w, 404, "not found", "job does not exist")
			return
		}
		problem(w, 500, "lookup failed", e.Error())
		return
	}
	after, limit, ok := eventPosition(w, r)
	if !ok {
		return
	}
	page, e := s.store.ListEvents(r.Context(), r.PathValue("id"), after, limit)
	if e != nil {
		problem(w, 500, "event lookup failed", e.Error())
		return
	}
	jsonResponse(w, 200, page)
}
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	if _, e := s.store.Get(r.Context(), r.PathValue("id")); e != nil {
		if errors.Is(e, jobs.ErrNotFound) {
			problem(w, 404, "not found", "job does not exist")
			return
		}
		problem(w, 500, "lookup failed", e.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		problem(w, 500, "stream unavailable", "response streaming is unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	after, _, ok := eventPosition(w, r)
	if !ok {
		return
	}
	if after == 0 && r.Header.Get("Last-Event-ID") != "" {
		var e error
		after, e = strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
		if e != nil || after < 0 {
			problem(w, 400, "invalid event position", "Last-Event-ID must be a non-negative integer")
			return
		}
	}
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	send := func() error {
		page, e := s.store.ListEvents(r.Context(), r.PathValue("id"), after, 200)
		if e != nil {
			return e
		}
		for _, event := range page.Items {
			body, _ := json.Marshal(event)
			if _, e = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, body); e != nil {
				return e
			}
			after = event.ID
		}
		if len(page.Items) > 0 {
			flusher.Flush()
		}
		return nil
	}
	if send() != nil {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			if send() != nil {
				return
			}
		case <-heartbeat.C:
			if _, e := fmt.Fprint(w, ": heartbeat\n\n"); e != nil {
				return
			}
			flusher.Flush()
		}
	}
}
func eventPosition(w http.ResponseWriter, r *http.Request) (int64, int, bool) {
	after, limit := int64(0), 100
	var err error
	if value := r.URL.Query().Get("after"); value != "" {
		after, err = strconv.ParseInt(value, 10, 64)
		if err != nil || after < 0 {
			problem(w, 400, "invalid event position", "after must be a non-negative integer")
			return 0, 0, false
		}
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 200 {
			problem(w, 400, "invalid page size", "limit must be between 1 and 200")
			return 0, 0, false
		}
	}
	return after, limit, true
}
func (s *Server) requeue(w http.ResponseWriter, r *http.Request) {
	e := s.store.RequeueDead(r.Context(), r.PathValue("id"))
	if errors.Is(e, jobs.ErrNotFound) {
		problem(w, 409, "cannot requeue", "job is absent or not dead")
		return
	}
	if e != nil {
		problem(w, 500, "requeue failed", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	var req jobs.CancelRequest
	if r.ContentLength > 0 {
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		if e := d.Decode(&req); e != nil {
			problem(w, 400, "invalid request", e.Error())
			return
		}
	}
	e := s.store.Cancel(r.Context(), r.PathValue("id"), req.Reason)
	if errors.Is(e, jobs.ErrNotFound) {
		problem(w, 409, "cannot cancel", "job is absent or already finished")
		return
	}
	if e != nil {
		problem(w, 500, "cancel failed", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) submitWorkflow(w http.ResponseWriter, r *http.Request) {
	var req workflows.SubmitRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	d.DisallowUnknownFields()
	if e := d.Decode(&req); e != nil {
		problem(w, 400, "invalid request", e.Error())
		return
	}
	if p, ok := auth.FromContext(r.Context()); ok {
		req.OwnerID = p.ID
	}
	req.Traceparent = tracing.Traceparent(r.Context())
	workflow, e := s.workflows.Submit(r.Context(), req)
	if e != nil {
		problem(w, 400, "workflow rejected", e.Error())
		return
	}
	w.Header().Set("Location", "/v1/workflows/"+workflow.ID)
	jsonResponse(w, 201, workflow)
}
func (s *Server) getWorkflow(w http.ResponseWriter, r *http.Request) {
	workflow, e := s.workflows.Get(r.Context(), r.PathValue("id"))
	if errors.Is(e, workflows.ErrNotFound) {
		problem(w, 404, "not found", "workflow does not exist")
		return
	}
	if e != nil {
		problem(w, 500, "lookup failed", e.Error())
		return
	}
	jsonResponse(w, 200, workflow)
}
func (s *Server) listWorkflows(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	after, e := parseOptionalTime(r.URL.Query().Get("created_after"))
	if e != nil {
		problem(w, 400, "invalid created_after", e.Error())
		return
	}
	before, e := parseOptionalTime(r.URL.Query().Get("created_before"))
	if e != nil {
		problem(w, 400, "invalid created_before", e.Error())
		return
	}
	ownerID := ""
	if p, ok := auth.FromContext(r.Context()); ok && p.Role != "admin" {
		ownerID = p.ID
	}
	page, e := s.workflows.List(r.Context(), workflows.ListFilter{Name: r.URL.Query().Get("name"), Status: r.URL.Query().Get("status"), Cursor: r.URL.Query().Get("cursor"), Limit: limit, CreatedAfter: after, CreatedBefore: before, OwnerID: ownerID})
	if e != nil {
		if errors.Is(e, pagination.ErrInvalidCursor) {
			problem(w, 400, "invalid cursor", e.Error())
			return
		}
		problem(w, 500, "lookup failed", e.Error())
		return
	}
	jsonResponse(w, 200, page)
}
func (s *Server) cancelWorkflow(w http.ResponseWriter, r *http.Request) {
	var req jobs.CancelRequest
	if r.ContentLength > 0 {
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		if e := d.Decode(&req); e != nil {
			problem(w, 400, "invalid request", e.Error())
			return
		}
	}
	e := s.workflows.Cancel(r.Context(), r.PathValue("id"), req.Reason)
	if errors.Is(e, workflows.ErrNotFound) {
		problem(w, 409, "cannot cancel", "workflow is absent or already finished")
		return
	}
	if e != nil {
		problem(w, 500, "cancel failed", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) createSchedule(w http.ResponseWriter, r *http.Request) {
	var req schedules.CreateRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if e := d.Decode(&req); e != nil || req.Name == "" || req.Type == "" || req.IntervalSeconds < 1 || !json.Valid(req.Payload) {
		problem(w, 400, "invalid schedule", "name, type, payload, and a positive interval_seconds are required")
		return
	}
	if p, ok := auth.FromContext(r.Context()); ok {
		req.OwnerID = p.ID
	}
	x, e := s.schedules.Create(r.Context(), req)
	if e != nil {
		problem(w, 400, "schedule rejected", e.Error())
		return
	}
	jsonResponse(w, 201, x)
}
func (s *Server) listSchedules(w http.ResponseWriter, r *http.Request) {
	xs, e := s.schedules.List(r.Context())
	if e != nil {
		problem(w, 500, "schedule lookup failed", e.Error())
		return
	}
	jsonResponse(w, 200, xs)
}
func (s *Server) enableSchedule(w http.ResponseWriter, r *http.Request) {
	s.setScheduleEnabled(w, r, true)
}
func (s *Server) disableSchedule(w http.ResponseWriter, r *http.Request) {
	s.setScheduleEnabled(w, r, false)
}
func (s *Server) setScheduleEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	e := s.schedules.SetEnabled(r.Context(), r.PathValue("id"), enabled)
	if schedules.IsNotFound(e) {
		problem(w, 404, "not found", "schedule does not exist")
		return
	}
	if e != nil {
		problem(w, 500, "schedule update failed", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) listWorkers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT id,queue,queues,capabilities,concurrency,started_at,last_heartbeat_at,metadata,state,draining_at,(last_heartbeat_at>now()-interval '30 seconds' AND state='running') healthy FROM workers ORDER BY last_heartbeat_at DESC`)
	if err != nil {
		problem(w, 500, "worker lookup failed", err.Error())
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, queue, state string
		var queues, capabilities []string
		var concurrency int
		var started, heartbeat time.Time
		var metadata json.RawMessage
		var drainingAt *time.Time
		var healthy bool
		if err = rows.Scan(&id, &queue, &queues, &capabilities, &concurrency, &started, &heartbeat, &metadata, &state, &drainingAt, &healthy); err != nil {
			problem(w, 500, "worker lookup failed", err.Error())
			return
		}
		items = append(items, map[string]any{"id": id, "queue": queue, "queues": queues, "capabilities": capabilities, "concurrency": concurrency, "started_at": started, "last_heartbeat_at": heartbeat, "metadata": metadata, "state": state, "draining_at": drainingAt, "healthy": healthy})
	}
	jsonResponse(w, 200, map[string]any{"items": items})
}
func (s *Server) listQueues(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT DISTINCT q.queue,COALESCE(c.paused,false),c.max_concurrency,(SELECT count(*) FROM jobs j WHERE j.queue=q.queue AND j.status='running') running FROM (SELECT queue FROM jobs UNION SELECT queue FROM queue_controls) q LEFT JOIN queue_controls c ON c.queue=q.queue ORDER BY q.queue`)
	if err != nil {
		problem(w, 500, "queue lookup failed", err.Error())
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var queue string
		var paused bool
		var max *int
		var running int
		if err = rows.Scan(&queue, &paused, &max, &running); err != nil {
			problem(w, 500, "queue lookup failed", err.Error())
			return
		}
		items = append(items, map[string]any{"queue": queue, "paused": paused, "max_concurrency": max, "running": running})
	}
	jsonResponse(w, 200, map[string]any{"items": items})
}
func (s *Server) updateQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paused         bool `json:"paused"`
		MaxConcurrency *int `json:"max_concurrency"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || (req.MaxConcurrency != nil && *req.MaxConcurrency < 1) {
		problem(w, 400, "invalid queue control", "max_concurrency must be positive or null")
		return
	}
	_, err := s.db.Exec(r.Context(), `INSERT INTO queue_controls(queue,paused,max_concurrency) VALUES($1,$2,$3) ON CONFLICT(queue) DO UPDATE SET paused=EXCLUDED.paused,max_concurrency=EXCLUDED.max_concurrency,updated_at=now()`, r.PathValue("queue"), req.Paused, req.MaxConcurrency)
	if err != nil {
		problem(w, 500, "queue update failed", err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) updateQuota(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SubmissionsPerMinute int `json:"submissions_per_minute"`
		MaxConcurrentJobs    int `json:"max_concurrent_jobs"`
		MaxPayloadBytes      int `json:"max_payload_bytes"`
		MaxSchedules         int `json:"max_schedules"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if d.Decode(&req) != nil || req.SubmissionsPerMinute < 1 || req.MaxConcurrentJobs < 1 || req.MaxPayloadBytes < 1 || req.MaxSchedules < 1 {
		problem(w, 400, "invalid quota", "all quota values must be positive")
		return
	}
	_, err := s.db.Exec(r.Context(), `INSERT INTO tenant_quotas(principal_id,submissions_per_minute,max_concurrent_jobs,max_payload_bytes,max_schedules) VALUES($1,$2,$3,$4,$5) ON CONFLICT(principal_id) DO UPDATE SET submissions_per_minute=EXCLUDED.submissions_per_minute,max_concurrent_jobs=EXCLUDED.max_concurrent_jobs,max_payload_bytes=EXCLUDED.max_payload_bytes,max_schedules=EXCLUDED.max_schedules,updated_at=now()`, r.PathValue("id"), req.SubmissionsPerMinute, req.MaxConcurrentJobs, req.MaxPayloadBytes, req.MaxSchedules)
	if err != nil {
		problem(w, 400, "quota update failed", err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) createPrincipal(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name, Role string }
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || req.Name == "" || (req.Role != "viewer" && req.Role != "operator" && req.Role != "admin") {
		problem(w, 400, "invalid principal", "name and role (viewer, operator, or admin) are required")
		return
	}
	var p auth.Principal
	if err := s.db.QueryRow(r.Context(), `INSERT INTO principals(name,role) VALUES($1,$2) RETURNING id,name,role::text`, req.Name, req.Role).Scan(&p.ID, &p.Name, &p.Role); err != nil {
		problem(w, 409, "principal creation failed", err.Error())
		return
	}
	jsonResponse(w, 201, p)
}
func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || req.Name == "" {
		problem(w, 400, "invalid API key", "name is required")
		return
	}
	key, token, err := auth.CreateKey(r.Context(), s.db, r.PathValue("id"), req.Name, req.ExpiresAt)
	if err != nil {
		problem(w, 400, "API key creation failed", err.Error())
		return
	}
	jsonResponse(w, 201, map[string]any{"key": key, "token": token, "warning": "the token is shown only once"})
}
func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT k.id,k.name,k.prefix,k.expires_at,k.last_used_at,k.revoked_at,k.created_at,p.id,p.name,p.role::text FROM api_keys k JOIN principals p ON p.id=k.principal_id ORDER BY k.created_at DESC LIMIT 200`)
	if err != nil {
		problem(w, 500, "API key lookup failed", err.Error())
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var key auth.Key
		var principal auth.Principal
		if err = rows.Scan(&key.ID, &key.Name, &key.Prefix, &key.ExpiresAt, &key.LastUsedAt, &key.RevokedAt, &key.CreatedAt, &principal.ID, &principal.Name, &principal.Role); err != nil {
			problem(w, 500, "API key lookup failed", err.Error())
			return
		}
		items = append(items, map[string]any{"key": key, "principal": principal})
	}
	if err = rows.Err(); err != nil {
		problem(w, 500, "API key lookup failed", err.Error())
		return
	}
	jsonResponse(w, 200, map[string]any{"items": items})
}
func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if err := auth.RevokeKey(r.Context(), s.db, r.PathValue("id")); err != nil {
		problem(w, 404, "not found", "API key does not exist or is already revoked")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) auditLog(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT id,principal_id,action,resource_type,resource_id,method,path,status,remote_addr,created_at FROM audit_log ORDER BY id DESC LIMIT 200`)
	if err != nil {
		problem(w, 500, "audit lookup failed", err.Error())
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var principalID, resourceID, remote *string
		var action, resource, method, path string
		var status int
		var created time.Time
		if err = rows.Scan(&id, &principalID, &action, &resource, &resourceID, &method, &path, &status, &remote, &created); err != nil {
			problem(w, 500, "audit lookup failed", err.Error())
			return
		}
		items = append(items, map[string]any{"id": id, "principal_id": principalID, "action": action, "resource_type": resource, "resource_id": resourceID, "method": method, "path": path, "status": status, "remote_addr": remote, "created_at": created})
	}
	jsonResponse(w, 200, map[string]any{"items": items})
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, title, detail string) {
	jsonResponse(w, status, map[string]any{"title": title, "detail": detail, "status": status})
}
func parseOptionalTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("http request", "method", r.Method, "path", strings.Split(r.URL.Path, "?")[0], "duration_ms", time.Since(start).Milliseconds())
	})
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
