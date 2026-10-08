package artifacts

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	DB                                                     *pgxpool.Pool
	Endpoint, PublicEndpoint, AccessKey, SecretKey, Bucket string
	MaxBytes                                               int64
	Retention                                              time.Duration
	Client                                                 *http.Client
}

type Artifact struct {
	ID          string    `json:"id"`
	JobID       string    `json:"job_id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	SHA256      string    `json:"sha256"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s Store) Enabled() bool { return s.Endpoint != "" && s.Bucket != "" }
func (s Store) EnsureBucket(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	req, err := s.request(ctx, http.MethodPut, s.Endpoint, "/"+s.Bucket, nil, "")
	if err != nil {
		return err
	}
	res, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("create artifact bucket: %s: %s", res.Status, string(b))
	}
	return nil
}
func (s Store) Put(ctx context.Context, jobID, name, contentType string, body []byte) (Artifact, error) {
	if int64(len(body)) > s.MaxBytes {
		return Artifact{}, fmt.Errorf("artifact exceeds %d byte limit", s.MaxBytes)
	}
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\") {
		return Artifact{}, errors.New("artifact name must be a simple 1-255 character filename")
	}
	key := path.Join("jobs", jobID, fmt.Sprintf("%d-%s", time.Now().UnixNano(), name))
	req, err := s.request(ctx, http.MethodPut, s.Endpoint, "/"+s.Bucket+"/"+key, body, contentType)
	if err != nil {
		return Artifact{}, err
	}
	res, err := s.client().Do(req)
	if err != nil {
		return Artifact{}, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return Artifact{}, fmt.Errorf("artifact upload: %s: %s", res.Status, string(b))
	}
	sum := sha256.Sum256(body)
	expires := time.Now().Add(s.Retention)
	var a Artifact
	err = s.DB.QueryRow(ctx, `INSERT INTO job_artifacts(job_id,name,content_type,object_key,size_bytes,sha256,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,job_id,name,content_type,size_bytes,sha256,created_at,expires_at`, jobID, name, contentType, key, len(body), hex.EncodeToString(sum[:]), expires).Scan(&a.ID, &a.JobID, &a.Name, &a.ContentType, &a.SizeBytes, &a.SHA256, &a.CreatedAt, &a.ExpiresAt)
	return a, err
}
func (s Store) List(ctx context.Context, jobID string) ([]Artifact, error) {
	rows, err := s.DB.Query(ctx, `SELECT id,job_id,name,content_type,size_bytes,sha256,created_at,expires_at FROM job_artifacts WHERE job_id=$1 AND expires_at>now() ORDER BY created_at DESC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artifact{}
	for rows.Next() {
		var a Artifact
		if err = rows.Scan(&a.ID, &a.JobID, &a.Name, &a.ContentType, &a.SizeBytes, &a.SHA256, &a.CreatedAt, &a.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s Store) DownloadURL(ctx context.Context, jobID, id string, valid time.Duration) (string, error) {
	var key string
	err := s.DB.QueryRow(ctx, `SELECT object_key FROM job_artifacts WHERE id=$1 AND job_id=$2 AND expires_at>now()`, id, jobID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", pgx.ErrNoRows
	}
	if err != nil {
		return "", err
	}
	return s.presign(s.PublicEndpoint, "/"+s.Bucket+"/"+key, valid), nil
}
func (s Store) Cleanup(ctx context.Context) error {
	rows, err := s.DB.Query(ctx, `DELETE FROM job_artifacts WHERE expires_at<=now() RETURNING object_key`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if rows.Scan(&key) == nil {
			req, e := s.request(ctx, http.MethodDelete, s.Endpoint, "/"+s.Bucket+"/"+key, nil, "")
			if e == nil {
				res, e := s.client().Do(req)
				if e == nil {
					res.Body.Close()
				}
			}
		}
	}
	return rows.Err()
}
func (s Store) RunCleanup(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		_ = s.Cleanup(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
func (s Store) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}
func hash(v []byte) string { x := sha256.Sum256(v); return hex.EncodeToString(x[:]) }
func sign(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(value))
	return h.Sum(nil)
}
func (s Store) signingKey(day string) []byte {
	return sign(sign(sign(sign([]byte("AWS4"+s.SecretKey), day), "us-east-1"), "s3"), "aws4_request")
}
func (s Store) request(ctx context.Context, method, endpoint, p string, body []byte, contentType string) (*http.Request, error) {
	u, err := endpointURL(endpoint, p)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	day := now.Format("20060102")
	stamp := now.Format("20060102T150405Z")
	payload := hash(body)
	r.Header.Set("Host", u.Host)
	r.Header.Set("X-Amz-Date", stamp)
	r.Header.Set("X-Amz-Content-Sha256", payload)
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	canonicalHeaders := "host:" + u.Host + "\n" + "x-amz-content-sha256:" + payload + "\n" + "x-amz-date:" + stamp + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonical := method + "\n" + u.EscapedPath() + "\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + payload
	scope := day + "/us-east-1/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hash([]byte(canonical))
	sig := hex.EncodeToString(sign(s.signingKey(day), toSign))
	r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.AccessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+sig)
	return r, nil
}
func (s Store) presign(endpoint, p string, valid time.Duration) string {
	if valid <= 0 || valid > time.Hour {
		valid = 15 * time.Minute
	}
	now := time.Now().UTC()
	day := now.Format("20060102")
	stamp := now.Format("20060102T150405Z")
	scope := day + "/us-east-1/s3/aws4_request"
	u, _ := endpointURL(endpoint, p)
	q := u.Query()
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", s.AccessKey+"/"+scope)
	q.Set("X-Amz-Date", stamp)
	q.Set("X-Amz-Expires", fmt.Sprint(int(valid.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	u.RawQuery = sortedQuery(q)
	canonical := "GET\n" + u.EscapedPath() + "\n" + u.RawQuery + "\nhost:" + u.Host + "\n\nhost\nUNSIGNED-PAYLOAD"
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hash([]byte(canonical))
	q.Set("X-Amz-Signature", hex.EncodeToString(sign(s.signingKey(day), toSign)))
	u.RawQuery = sortedQuery(q)
	return u.String()
}
func endpointURL(endpoint, p string) (*url.URL, error) {
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + p
	return u, nil
}
func sortedQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		for _, v := range q[k] {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.ReplaceAll(strings.Join(parts, "&"), "+", "%20")
}
