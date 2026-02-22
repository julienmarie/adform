package landing

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisBanditStore struct {
	ctx       context.Context
	client    *redis.Client
	keyPrefix string
}

type RedisBanditConfig struct {
	Addr      string
	Password  string
	DB        int
	KeyPrefix string
}

func OpenRedisBanditStore(cfg RedisBanditConfig) (*RedisBanditStore, error) {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		return nil, fmt.Errorf("redis addr is required")
	}
	prefix := strings.TrimSpace(cfg.KeyPrefix)
	if prefix == "" {
		prefix = "adform:landing:bandit"
	}
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: strings.TrimSpace(cfg.Password),
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisBanditStore{ctx: context.Background(), client: client, keyPrefix: prefix}, nil
}

func (s *RedisBanditStore) Close() error {
	return s.client.Close()
}

func (s *RedisBanditStore) EnsureArm(account, pageKey, blockKey, slot, arm string) error {
	key := s.armKey(account, pageKey, blockKey, slot, arm)
	pipe := s.client.TxPipeline()
	pipe.HSetNX(s.ctx, key, "impressions", 0)
	pipe.HSetNX(s.ctx, key, "clicks", 0)
	pipe.HSet(s.ctx, key, "updated_at", time.Now().UTC().Format(time.RFC3339))
	_, err := pipe.Exec(s.ctx)
	if err != nil {
		return fmt.Errorf("redis ensure arm: %w", err)
	}
	return nil
}

func (s *RedisBanditStore) IncrementImpression(account, pageKey, blockKey, slot, arm string) error {
	key := s.armKey(account, pageKey, blockKey, slot, arm)
	pipe := s.client.TxPipeline()
	pipe.HIncrBy(s.ctx, key, "impressions", 1)
	pipe.HSet(s.ctx, key, "updated_at", time.Now().UTC().Format(time.RFC3339))
	_, err := pipe.Exec(s.ctx)
	if err != nil {
		return fmt.Errorf("redis increment impression: %w", err)
	}
	return nil
}

func (s *RedisBanditStore) IncrementClick(account, pageKey, blockKey, slot, arm string) error {
	key := s.armKey(account, pageKey, blockKey, slot, arm)
	pipe := s.client.TxPipeline()
	pipe.HIncrBy(s.ctx, key, "clicks", 1)
	pipe.HSet(s.ctx, key, "updated_at", time.Now().UTC().Format(time.RFC3339))
	_, err := pipe.Exec(s.ctx)
	if err != nil {
		return fmt.Errorf("redis increment click: %w", err)
	}
	return nil
}

func (s *RedisBanditStore) Stats(account, pageKey, blockKey, slot string, arms []string) (map[string]ArmStats, error) {
	out := map[string]ArmStats{}
	pipe := s.client.Pipeline()
	cmds := make(map[string]*redis.SliceCmd, len(arms))
	for _, arm := range arms {
		arm = strings.TrimSpace(arm)
		if arm == "" {
			continue
		}
		out[arm] = ArmStats{ArmKey: arm}
		key := s.armKey(account, pageKey, blockKey, slot, arm)
		cmds[arm] = pipe.HMGet(s.ctx, key, "impressions", "clicks")
	}
	if _, err := pipe.Exec(s.ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("redis stats exec: %w", err)
	}
	for arm, cmd := range cmds {
		vals := cmd.Val()
		if len(vals) < 2 {
			continue
		}
		impressions := parseRedisInt(vals[0])
		clicks := parseRedisInt(vals[1])
		out[arm] = ArmStats{ArmKey: arm, Impressions: impressions, Clicks: clicks}
	}
	return out, nil
}

func (s *RedisBanditStore) armKey(account, pageKey, blockKey, slot, arm string) string {
	parts := []string{
		s.keyPrefix,
		strings.TrimSpace(account),
		strings.TrimSpace(pageKey),
		strings.TrimSpace(blockKey),
		strings.TrimSpace(slot),
		strings.TrimSpace(arm),
	}
	return strings.Join(parts, ":")
}

func parseRedisInt(v any) int64 {
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	if s == "" || s == "<nil>" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
