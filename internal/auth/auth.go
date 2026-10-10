package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
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
type sessionContextKey struct{}

const SessionCookieName = "fq_session"

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}

func IsAdmin(ctx context.Context) bool { p, ok := FromContext(ctx); return ok && p.Role == "admin" }

func SessionID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionContextKey{}).(string)
	return id, ok && id != ""
}

type Middleware struct{ DB *pgxpool.Pool }

func (m Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		var p Principal
		credentialID, sessionID, sessionAuth, err := m.authenticate(r, &p)
		if err != nil {
			unauthorized(w, http.StatusUnauthorized, err.Error())
			return
		}
		if sessionAuth && unsafeMethod(r.Method) {
			provided := sha256.Sum256([]byte(r.Header.Get("X-CSRF-Token")))
			var expected []byte
			if err = m.DB.QueryRow(r.Context(), `SELECT csrf_hash FROM web_sessions WHERE id=$1`, sessionID).Scan(&expected); err != nil || len(expected) != len(provided) || subtle.ConstantTimeCompare(expected, provided[:]) != 1 {
				unauthorized(w, http.StatusForbidden, "a valid CSRF token is required")
				return
			}
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
		if sessionAuth {
			_, _ = m.DB.Exec(r.Context(), `UPDATE web_sessions SET last_used_at=now() WHERE id=$1`, credentialID)
		} else {
			_, _ = m.DB.Exec(r.Context(), `UPDATE api_keys SET last_used_at=now() WHERE id=$1`, credentialID)
		}
		r = r.WithContext(context.WithValue(r.Context(), contextKey{}, p))
		if sessionAuth {
			r = r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, sessionID))
		}
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			_, _ = m.DB.Exec(context.WithoutCancel(r.Context()), `INSERT INTO audit_log(principal_id,action,resource_type,resource_id,method,path,status,remote_addr) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, p.ID, action(r), resourceType(r.URL.Path), r.PathValue("id"), r.Method, r.URL.Path, rw.status, r.RemoteAddr)
		}
	})
}

func (m Middleware) authenticate(r *http.Request, p *Principal) (credentialID, sessionID string, sessionAuth bool, err error) {
	authorization := r.Header.Get("Authorization")
	if strings.HasPrefix(authorization, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if token == "" {
			return "", "", false, errors.New("a Bearer API key is required")
		}
		sum := sha256.Sum256([]byte(token))
		err = m.DB.QueryRow(r.Context(), `SELECT p.id,p.name,p.role::text,k.id FROM api_keys k JOIN principals p ON p.id=k.principal_id WHERE k.key_hash=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())`, sum[:]).Scan(&p.ID, &p.Name, &p.Role, &credentialID)
		if err != nil {
			return "", "", false, errors.New("API key is invalid, expired, or revoked")
		}
		return credentialID, "", false, nil
	}
	cookie, cookieErr := r.Cookie(SessionCookieName)
	if cookieErr != nil || cookie.Value == "" {
		return "", "", false, errors.New("authentication is required")
	}
	sum := sha256.Sum256([]byte(cookie.Value))
	err = m.DB.QueryRow(r.Context(), `SELECT p.id,p.name,p.role::text,s.id FROM web_sessions s JOIN principals p ON p.id=s.principal_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now()`, sum[:]).Scan(&p.ID, &p.Name, &p.Role, &sessionID)
	if err != nil {
		return "", "", false, errors.New("web session is invalid, expired, or revoked")
	}
	return sessionID, sessionID, true, nil
}

func unsafeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
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
	if path == "/v1/session" {
		return true
	}
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

type WebSession struct {
	Principal Principal `json:"principal"`
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func CreateWebSession(ctx context.Context, db *pgxpool.Pool, principal Principal, duration time.Duration) (WebSession, string, error) {
	if duration <= 0 {
		duration = 8 * time.Hour
	}
	token, err := randomToken(32)
	if err != nil {
		return WebSession{}, "", err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return WebSession{}, "", err
	}
	tokenHash, csrfHash := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(csrf))
	expires := time.Now().UTC().Add(duration)
	_, _ = db.Exec(ctx, `DELETE FROM web_sessions WHERE expires_at<=now() OR revoked_at IS NOT NULL`)
	_, err = db.Exec(ctx, `INSERT INTO web_sessions(principal_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4)`, principal.ID, tokenHash[:], csrfHash[:], expires)
	return WebSession{Principal: principal, CSRFToken: csrf, ExpiresAt: expires}, token, err
}

func RotateSessionCSRF(ctx context.Context, db *pgxpool.Pool, sessionID string, principal Principal) (WebSession, error) {
	csrf, err := randomToken(32)
	if err != nil {
		return WebSession{}, err
	}
	csrfHash := sha256.Sum256([]byte(csrf))
	var expires time.Time
	err = db.QueryRow(ctx, `UPDATE web_sessions SET csrf_hash=$2,last_used_at=now() WHERE id=$1 AND revoked_at IS NULL AND expires_at>now() RETURNING expires_at`, sessionID, csrfHash[:]).Scan(&expires)
	return WebSession{Principal: principal, CSRFToken: csrf, ExpiresAt: expires}, err
}

func RevokeWebSession(ctx context.Context, db *pgxpool.Pool, sessionID string) error {
	_, err := db.Exec(ctx, `UPDATE web_sessions SET revoked_at=now() WHERE id=$1`, sessionID)
	return err
}

func SetSessionCookie(w http.ResponseWriter, token string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: token, Path: "/", Expires: expires, MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode})
}

func randomToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
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
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
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
