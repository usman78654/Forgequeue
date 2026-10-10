package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var files embed.FS

const advisoryLockID int64 = 734627581926

var developmentPrincipals = []struct {
	Name, Role, Token, Prefix string
}{
	{"local-admin", "admin", "fq_admin_dev", "fq_adm"},
	{"local-operator", "operator", "fq_operator_dev", "fq_ope"},
	{"local-viewer", "viewer", "fq_viewer_dev", "fq_vie"},
}

type Migration struct {
	Version             int64
	Name, SQL, Checksum string
}

func Load() ([]Migration, error) {
	entries, err := fs.ReadDir(files, "sql")
	if err != nil {
		return nil, err
	}
	var result []Migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid migration version %q: %w", entry.Name(), err)
		}
		body, err := files.ReadFile("sql/" + entry.Name())
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		result = append(result, Migration{Version: version, Name: entry.Name(), SQL: string(body), Checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	for i := 1; i < len(result); i++ {
		if result[i-1].Version == result[i].Version {
			return nil, fmt.Errorf("duplicate migration version %d", result[i].Version)
		}
	}
	return result, nil
}

func Run(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := Load()
	if err != nil {
		return err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockID)
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version bigint PRIMARY KEY,name text NOT NULL,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	for _, migration := range migrations {
		var checksum string
		err = conn.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE version=$1`, migration.Version).Scan(&checksum)
		if err == nil {
			if checksum != migration.Checksum {
				return fmt.Errorf("migration %d checksum changed", migration.Version)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		baselined, err := baselineExisting(ctx, conn, migration.Version)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if !baselined {
			if _, err = tx.Exec(ctx, migration.SQL); err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("apply %s: %w", migration.Name, err)
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version,name,checksum,applied_at) VALUES($1,$2,$3,$4)`, migration.Version, migration.Name, migration.Checksum, time.Now().UTC()); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// SeedDevelopment installs deterministic credentials for local demos. Callers must
// opt in explicitly; the forward cleanup migration removes legacy seeded rows.
func SeedDevelopment(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, principal := range developmentPrincipals {
		var principalID string
		if err = tx.QueryRow(ctx, `INSERT INTO principals(name,role) VALUES($1,$2::principal_role) ON CONFLICT(name) DO UPDATE SET role=EXCLUDED.role RETURNING id`, principal.Name, principal.Role).Scan(&principalID); err != nil {
			return fmt.Errorf("seed development principal %q: %w", principal.Name, err)
		}
		sum := sha256.Sum256([]byte(principal.Token))
		if _, err = tx.Exec(ctx, `INSERT INTO api_keys(principal_id,name,key_hash,prefix) VALUES($1,'development',$2,$3) ON CONFLICT(principal_id,name) DO UPDATE SET key_hash=EXCLUDED.key_hash,prefix=EXCLUDED.prefix,expires_at=NULL,revoked_at=NULL`, principalID, sum[:], principal.Prefix); err != nil {
			return fmt.Errorf("seed development key for %q: %w", principal.Name, err)
		}
	}
	return tx.Commit(ctx)
}

func baselineExisting(ctx context.Context, conn *pgxpool.Conn, version int64) (bool, error) {
	table := ""
	switch version {
	case 1:
		table = "jobs"
	case 2:
		table = "workflows"
	default:
		return false, nil
	}
	var exists bool
	err := conn.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists)
	return exists, err
}
