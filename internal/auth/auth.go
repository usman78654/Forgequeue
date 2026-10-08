package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Principal struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type contextKey struct{}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}

func IsAdmin(ctx context.Context) bool { p, ok := FromContext(ctx); return ok && p.Role == "admin" }

type Middleware struct{ DB *pgxpool.Pool }

func (m Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" || token == r.Header.Get("Authorization") {
			unauthorized(w, http.StatusUnauthorized, "a Bearer API key is required")
			return
		}
		sum := sha256.Sum256([]byte(token))
		var p Principal
		var keyID string
		err := m.DB.QueryRow(r.Context(), `SELECT p.id,p.name,p.role::text,k.id FROM api_keys k JOIN principals p ON p.id=k.principal_id WHERE k.key_hash=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())`, sum[:]).Scan(&p.ID, &p.Name, &p.Role, &keyID)
		if err != nil {
			unauthorized(w, http.StatusUnauthorized, "API key is invalid, expired, or revoked")
			return
		}
		if !allowed(p.Role, r.Method, r.URL.Path) {
			unauthorized(w, http.StatusForbidden, "this role cannot perform that operation")
			return
		}
		if p.Role != "admin" {
			owned, err := m.owns(r.Context(), p.ID, r.URL.Path)
			if err != nil || !owned {
				unauthorized(w, http.StatusForbidden, "the resource belongs to another principal")
				return
			}
		}
		_, _ = m.DB.Exec(r.Context(), `UPDATE api_keys SET last_used_at=now() WHERE id=$1`, keyID)
		r = r.WithContext(context.WithValue(r.Context(), contextKey{}, p))
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			_, _ = m.DB.Exec(context.WithoutCancel(r.Context()), `INSERT INTO audit_log(principal_id,action,resource_type,resource_id,method,path,status,remote_addr) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, p.ID, action(r), resourceType(r.URL.Path), r.PathValue("id"), r.Method, r.URL.Path, rw.status, r.RemoteAddr)
		}
	})
}

func (m Middleware) owns(ctx context.Context, principalID, path string) (bool, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "v1" || (parts[1] != "jobs" && parts[1] != "workflows") {
		return true, nil
	}
	table := parts[1]
	var owned bool
	err := m.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id=$1::uuid AND (owner_id=$2 OR owner_id IS NULL))`, parts[2], principalID).Scan(&owned)
	return owned, err
}

func publicPath(path string) bool {
	return path == "/" || path == "/healthz" || path == "/readyz" || path == "/metrics" || strings.HasPrefix(path, "/app/")
}

func allowed(role, method, path string) bool {
	if role == "admin" {
		return true
	}
	if strings.HasPrefix(path, "/v1/admin/") {
		return false
	}
	if role == "viewer" {
		return method == http.MethodGet || method == http.MethodHead
	}
	return role == "operator"
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func unauthorized(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "title": http.StatusText(status), "detail": detail})
}

func action(r *http.Request) string {
	return strings.ToLower(r.Method) + " " + resourceType(r.URL.Path)
}
func resourceType(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 1 {
		return parts[1]
	}
	return "system"
}

type Key struct {
	ID, Name, Prefix                 string
	ExpiresAt, LastUsedAt, RevokedAt *time.Time
	CreatedAt                        time.Time
}

func CreateKey(ctx context.Context, db *pgxpool.Pool, principalID, name string, expiresAt *time.Time) (Key, string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return Key{}, "", err
	}
	token := "fq_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	var k Key
	err := db.QueryRow(ctx, `INSERT INTO api_keys(principal_id,name,key_hash,prefix,expires_at) VALUES($1,$2,$3,$4,$5) RETURNING id,name,prefix,expires_at,last_used_at,revoked_at,created_at`, principalID, name, sum[:], token[:10], expiresAt).Scan(&k.ID, &k.Name, &k.Prefix, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt, &k.CreatedAt)
	return k, token, err
}

func RevokeKey(ctx context.Context, db *pgxpool.Pool, id string) error {
	tag, err := db.Exec(ctx, `UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}
