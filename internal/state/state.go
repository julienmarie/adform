package state

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrLocked = errors.New("account is already locked")

type ResourceRow struct {
	AccountName        string
	Kind               string
	LogicalKey         string
	MetaID             string
	LastAppliedHash    string
	LastSeenRemoteHash string
	CreatedAt          string
	UpdatedAt          string
}

type FeedCacheRow struct {
	AccountName string
	FeedURL     string
	FetchedAt   string
	PayloadJSON string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.Migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS resources (
			account_name TEXT NOT NULL,
			kind TEXT NOT NULL,
			logical_key TEXT NOT NULL,
			meta_id TEXT,
			last_applied_hash TEXT,
			last_seen_remote_hash TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (account_name, kind, logical_key)
		)`,
		`CREATE TABLE IF NOT EXISTS locks (
			account_name TEXT PRIMARY KEY,
			locked_at TEXT NOT NULL,
			locked_by TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS apply_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_name TEXT NOT NULL,
			applied_at TEXT NOT NULL,
			actor TEXT NOT NULL,
			plan_json TEXT NOT NULL,
			result_json TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_name TEXT NOT NULL,
			created_at TEXT NOT NULL,
			stats_json TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS account_meta (
			account_name TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (account_name, key)
		)`,
		`CREATE TABLE IF NOT EXISTS feed_cache (
			account_name TEXT NOT NULL,
			feed_url TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			PRIMARY KEY (account_name, feed_url)
		)`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) AcquireLock(accountName, actor string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO locks(account_name, locked_at, locked_by) VALUES(?, ?, ?)`, accountName, now, actor)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "UNIQUE") {
		return ErrLocked
	}
	return fmt.Errorf("acquire lock: %w", err)
}

func (s *Store) ReleaseLock(accountName string) error {
	_, err := s.db.Exec(`DELETE FROM locks WHERE account_name = ?`, accountName)
	if err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}

func (s *Store) UpsertResource(row ResourceRow) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if row.CreatedAt == "" {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	_, err := s.db.Exec(`
		INSERT INTO resources(account_name, kind, logical_key, meta_id, last_applied_hash, last_seen_remote_hash, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_name, kind, logical_key) DO UPDATE SET
			meta_id=excluded.meta_id,
			last_applied_hash=excluded.last_applied_hash,
			last_seen_remote_hash=excluded.last_seen_remote_hash,
			updated_at=excluded.updated_at
	`, row.AccountName, row.Kind, row.LogicalKey, row.MetaID, row.LastAppliedHash, row.LastSeenRemoteHash, row.CreatedAt, row.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert resource: %w", err)
	}
	return nil
}

func (s *Store) GetResource(accountName, kind, logicalKey string) (*ResourceRow, error) {
	var row ResourceRow
	err := s.db.QueryRow(`
		SELECT account_name, kind, logical_key, meta_id, last_applied_hash, last_seen_remote_hash, created_at, updated_at
		FROM resources WHERE account_name=? AND kind=? AND logical_key=?
	`, accountName, kind, logicalKey).Scan(
		&row.AccountName,
		&row.Kind,
		&row.LogicalKey,
		&row.MetaID,
		&row.LastAppliedHash,
		&row.LastSeenRemoteHash,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get resource: %w", err)
	}
	return &row, nil
}

func (s *Store) ListResources(accountName string) ([]ResourceRow, error) {
	rows, err := s.db.Query(`
		SELECT account_name, kind, logical_key, meta_id, last_applied_hash, last_seen_remote_hash, created_at, updated_at
		FROM resources WHERE account_name=?
	`, accountName)
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}
	defer rows.Close()

	out := make([]ResourceRow, 0)
	for rows.Next() {
		var row ResourceRow
		if err := rows.Scan(
			&row.AccountName,
			&row.Kind,
			&row.LogicalKey,
			&row.MetaID,
			&row.LastAppliedHash,
			&row.LastSeenRemoteHash,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan resources: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resources: %w", err)
	}
	return out, nil
}

func (s *Store) InsertApplyLog(accountName, actor, planJSON, resultJSON string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO apply_log(account_name, applied_at, actor, plan_json, result_json)
		VALUES(?, ?, ?, ?, ?)
	`, accountName, now, actor, planJSON, resultJSON)
	if err != nil {
		return fmt.Errorf("insert apply log: %w", err)
	}
	return nil
}

func (s *Store) InsertSnapshot(accountName, statsJSON string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO snapshots(account_name, created_at, stats_json)
		VALUES(?, ?, ?)
	`, accountName, now, statsJSON)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}
	return nil
}

func (s *Store) DeleteResource(accountName, kind, logicalKey string) error {
	_, err := s.db.Exec(`DELETE FROM resources WHERE account_name=? AND kind=? AND logical_key=?`, accountName, kind, logicalKey)
	if err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}
	return nil
}

func (s *Store) SetAccountMeta(accountName, key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO account_meta(account_name, key, value, updated_at)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(account_name, key) DO UPDATE SET
			value=excluded.value,
			updated_at=excluded.updated_at
	`, accountName, key, value, now)
	if err != nil {
		return fmt.Errorf("set account meta: %w", err)
	}
	return nil
}

func (s *Store) GetAccountMeta(accountName, key string) (string, error) {
	var value string
	err := s.db.QueryRow(`
		SELECT value FROM account_meta
		WHERE account_name=? AND key=?
	`, accountName, key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get account meta: %w", err)
	}
	return value, nil
}

func (s *Store) UpsertFeedCache(row FeedCacheRow) error {
	if strings.TrimSpace(row.FetchedAt) == "" {
		row.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`
		INSERT INTO feed_cache(account_name, feed_url, fetched_at, payload_json)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(account_name, feed_url) DO UPDATE SET
			fetched_at=excluded.fetched_at,
			payload_json=excluded.payload_json
	`, row.AccountName, row.FeedURL, row.FetchedAt, row.PayloadJSON)
	if err != nil {
		return fmt.Errorf("upsert feed cache: %w", err)
	}
	return nil
}

func (s *Store) GetFeedCache(accountName, feedURL string) (*FeedCacheRow, error) {
	var row FeedCacheRow
	err := s.db.QueryRow(`
		SELECT account_name, feed_url, fetched_at, payload_json
		FROM feed_cache
		WHERE account_name=? AND feed_url=?
	`, accountName, feedURL).Scan(&row.AccountName, &row.FeedURL, &row.FetchedAt, &row.PayloadJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get feed cache: %w", err)
	}
	return &row, nil
}
