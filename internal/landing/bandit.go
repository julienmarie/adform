package landing

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type BanditStore struct {
	db *sql.DB
}

type BanditCounterStore interface {
	Close() error
	EnsureArm(account, pageKey, blockKey, slot, arm string) error
	IncrementImpression(account, pageKey, blockKey, slot, arm string) error
	IncrementClick(account, pageKey, blockKey, slot, arm string) error
	Stats(account, pageKey, blockKey, slot string, arms []string) (map[string]ArmStats, error)
}

func OpenSQLiteBanditStore(path string) (*BanditStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &BanditStore{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func OpenBanditStore(path string) (*BanditStore, error) {
	return OpenSQLiteBanditStore(path)
}

func (s *BanditStore) Close() error {
	return s.db.Close()
}

func (s *BanditStore) migrate() error {
	stmt := `CREATE TABLE IF NOT EXISTS bandit_arms (
		account_name TEXT NOT NULL,
		page_key TEXT NOT NULL,
		block_key TEXT NOT NULL,
		slot TEXT NOT NULL,
		arm_key TEXT NOT NULL,
		impressions INTEGER NOT NULL DEFAULT 0,
		clicks INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (account_name, page_key, block_key, slot, arm_key)
	)`
	if _, err := s.db.Exec(stmt); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func (s *BanditStore) EnsureArm(account, pageKey, blockKey, slot, arm string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO bandit_arms(account_name, page_key, block_key, slot, arm_key, impressions, clicks, updated_at)
		VALUES(?, ?, ?, ?, ?, 0, 0, ?)
		ON CONFLICT(account_name, page_key, block_key, slot, arm_key) DO UPDATE SET updated_at=excluded.updated_at
	`, account, pageKey, blockKey, slot, arm, now)
	if err != nil {
		return fmt.Errorf("ensure arm: %w", err)
	}
	return nil
}

func (s *BanditStore) IncrementImpression(account, pageKey, blockKey, slot, arm string) error {
	if err := s.EnsureArm(account, pageKey, blockKey, slot, arm); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		UPDATE bandit_arms
		SET impressions=impressions+1, updated_at=?
		WHERE account_name=? AND page_key=? AND block_key=? AND slot=? AND arm_key=?
	`, time.Now().UTC().Format(time.RFC3339), account, pageKey, blockKey, slot, arm)
	if err != nil {
		return fmt.Errorf("increment impression: %w", err)
	}
	return nil
}

func (s *BanditStore) IncrementClick(account, pageKey, blockKey, slot, arm string) error {
	if err := s.EnsureArm(account, pageKey, blockKey, slot, arm); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		UPDATE bandit_arms
		SET clicks=clicks+1, updated_at=?
		WHERE account_name=? AND page_key=? AND block_key=? AND slot=? AND arm_key=?
	`, time.Now().UTC().Format(time.RFC3339), account, pageKey, blockKey, slot, arm)
	if err != nil {
		return fmt.Errorf("increment click: %w", err)
	}
	return nil
}

func (s *BanditStore) Stats(account, pageKey, blockKey, slot string, arms []string) (map[string]ArmStats, error) {
	out := map[string]ArmStats{}
	for _, arm := range arms {
		out[arm] = ArmStats{ArmKey: arm}
	}
	rows, err := s.db.Query(`
		SELECT arm_key, impressions, clicks
		FROM bandit_arms
		WHERE account_name=? AND page_key=? AND block_key=? AND slot=?
	`, account, pageKey, blockKey, slot)
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var arm string
		var impressions, clicks int64
		if err := rows.Scan(&arm, &impressions, &clicks); err != nil {
			return nil, fmt.Errorf("scan stats: %w", err)
		}
		if _, ok := out[arm]; ok {
			out[arm] = ArmStats{ArmKey: arm, Impressions: impressions, Clicks: clicks}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stats: %w", err)
	}
	return out, nil
}

func ChooseArm(arms []HeroArm, stats map[string]ArmStats, minImpressions int, controlMinShare float64) string {
	if len(arms) == 0 {
		return ""
	}
	if len(arms) == 1 {
		return strings.TrimSpace(arms[0].Key)
	}
	if minImpressions <= 0 {
		minImpressions = 200
	}
	if controlMinShare < 0 {
		controlMinShare = 0
	}
	if controlMinShare > 1 {
		controlMinShare = 1
	}

	armByKey := map[string]HeroArm{}
	totalImpressions := int64(0)
	controlKey := ""
	for _, arm := range arms {
		k := strings.TrimSpace(arm.Key)
		armByKey[k] = arm
		st := stats[k]
		totalImpressions += st.Impressions
		if controlKey == "" && strings.EqualFold(k, "control") {
			controlKey = k
		}
	}
	if controlKey == "" {
		controlKey = strings.TrimSpace(arms[0].Key)
	}
	if totalImpressions > 0 {
		controlShare := float64(stats[controlKey].Impressions) / float64(totalImpressions)
		if controlShare < controlMinShare {
			return controlKey
		}
	}

	underexposed := make([]string, 0)
	for _, arm := range arms {
		k := strings.TrimSpace(arm.Key)
		if stats[k].Impressions < int64(minImpressions) {
			underexposed = append(underexposed, k)
		}
	}
	if len(underexposed) > 0 {
		return underexposed[rand.Intn(len(underexposed))]
	}

	best := strings.TrimSpace(arms[0].Key)
	bestScore := -1.0
	for _, arm := range arms {
		k := strings.TrimSpace(arm.Key)
		st := stats[k]
		alpha := 1.0 + float64(st.Clicks)
		failures := st.Impressions - st.Clicks
		if failures < 0 {
			failures = 0
		}
		beta := 1.0 + float64(failures)
		s := sampleBeta(alpha, beta)
		if s > bestScore {
			bestScore = s
			best = k
		}
	}
	return best
}

func sampleBeta(alpha, beta float64) float64 {
	x := sampleGamma(alpha)
	y := sampleGamma(beta)
	if x+y == 0 {
		return 0.5
	}
	return x / (x + y)
}

func sampleGamma(k float64) float64 {
	if k <= 0 {
		return 0
	}
	if k < 1 {
		u := rand.Float64()
		return sampleGamma(k+1) * math.Pow(u, 1/k)
	}
	d := k - 1.0/3.0
	c := 1.0 / math.Sqrt(9*d)
	for {
		x := rand.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rand.Float64()
		if u < 1-0.0331*(x*x)*(x*x) {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}
