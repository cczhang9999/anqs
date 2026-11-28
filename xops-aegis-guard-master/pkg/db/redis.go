package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"xops-aegis-guard/config"

	redislib "github.com/redis/go-redis/v9"
)

const (
	RedisClusterMode   = "cluster"
	RedisSingletonMode = "singleton"
)

type RedisCommandClient interface {
	Get(context.Context, string) *redislib.StringCmd
	Set(context.Context, string, interface{}, time.Duration) *redislib.StatusCmd
	SetNX(context.Context, string, interface{}, time.Duration) *redislib.BoolCmd
	Del(context.Context, ...string) *redislib.IntCmd
	Eval(context.Context, string, []string, ...interface{}) *redislib.Cmd
	Close() error
}

var (
	redisClientMu      sync.RWMutex
	currentRedisClient RedisCommandClient
	redisProxyClient   RedisCommandClient = &redisClientProxy{}

	RedisClient        *redislib.Client
	RedisClusterClient *redislib.ClusterClient
)

var (
	_ RedisCommandClient = (*redislib.Client)(nil)
	_ RedisCommandClient = (*redislib.ClusterClient)(nil)
	_ RedisCommandClient = (*redisClientProxy)(nil)
)

func NewRedisClusterClient(conf *config.Redis) (*redislib.ClusterClient, error) {
	pool := redisPoolConfig(conf)
	client := redislib.NewClusterClient(&redislib.ClusterOptions{
		Addrs:        conf.Addrs,
		Username:     conf.Username,
		Password:     conf.Password,
		PoolSize:     pool.PoolSize,
		MinIdleConns: pool.MinIdleConns,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func NewRedisClient(conf *config.Redis) (*redislib.Client, error) {
	pool := redisPoolConfig(conf)
	client := redislib.NewClient(&redislib.Options{
		Addr:         conf.Addrs[0],
		Username:     conf.Username,
		Password:     conf.Password,
		DB:           conf.DB,
		PoolSize:     pool.PoolSize,
		MinIdleConns: pool.MinIdleConns,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func redisVaultCredentialsFile(conf *config.Redis) string {
	if path := strings.TrimSpace(os.Getenv("VAULT_CREDENTIALS_FILE")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("VAULT_REDIS_CREDENTIALS_FILE")); path != "" {
		return path
	}
	if conf != nil {
		return strings.TrimSpace(conf.VaultCredentialsFile)
	}
	return ""
}

func RedisVaultEnabled(conf config.Redis) bool {
	if redisVaultCredentialsFile(&conf) == "" {
		return false
	}
	if raw := strings.TrimSpace(os.Getenv("VAULT_ENABLED")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		return err != nil || enabled
	}
	return true
}

func InitRedis(conf config.Redis) error {
	if len(conf.Addrs) == 0 {
		return nil
	}
	if conf.Mode == "" {
		conf.Mode = RedisSingletonMode
	}
	if RedisVaultEnabled(conf) {
		conf.VaultCredentialsFile = redisVaultCredentialsFile(&conf)
		return initVaultRedis(conf)
	}
	next, err := newRedisCommandClient(&conf)
	if err != nil {
		return err
	}
	old := swapRedisClient(next)
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func newRedisCommandClient(conf *config.Redis) (RedisCommandClient, error) {
	switch conf.Mode {
	case RedisClusterMode:
		client, err := NewRedisClusterClient(conf)
		if err != nil {
			return nil, fmt.Errorf("connect redis cluster: %w", err)
		}
		return client, nil
	case RedisSingletonMode:
		client, err := NewRedisClient(conf)
		if err != nil {
			return nil, fmt.Errorf("connect redis: %w", err)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported redis mode %q", conf.Mode)
	}
}

func redisPoolConfig(conf *config.Redis) *config.RedisPool {
	if conf.Pool == nil {
		return &config.RedisPool{PoolSize: 10}
	}
	return conf.Pool
}

// RedisUniversalClient returns a stable proxy whose operations use the latest pool.
func RedisUniversalClient() RedisCommandClient { return redisProxyClient }

func swapRedisClient(next RedisCommandClient) RedisCommandClient {
	redisClientMu.Lock()
	old := currentRedisClient
	currentRedisClient = next
	RedisClient = nil
	RedisClusterClient = nil
	switch client := next.(type) {
	case *redislib.Client:
		RedisClient = client
	case *redislib.ClusterClient:
		RedisClusterClient = client
	}
	redisClientMu.Unlock()
	return old
}

func activeRedisClient() RedisCommandClient {
	redisClientMu.RLock()
	current := currentRedisClient
	redisClientMu.RUnlock()
	return current
}

func RedisClose() {
	stopVaultRedisWatcher()
	current := swapRedisClient(nil)
	if current != nil {
		_ = current.Close()
	}
}

var errRedisNotInitialized = errors.New("redis client is not initialized")

type redisClientProxy struct{}

func (p *redisClientProxy) Get(ctx context.Context, key string) *redislib.StringCmd {
	if client := activeRedisClient(); client != nil {
		return client.Get(ctx, key)
	}
	cmd := redislib.NewStringCmd(ctx)
	cmd.SetErr(errRedisNotInitialized)
	return cmd
}

func (p *redisClientProxy) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redislib.StatusCmd {
	if client := activeRedisClient(); client != nil {
		return client.Set(ctx, key, value, expiration)
	}
	cmd := redislib.NewStatusCmd(ctx)
	cmd.SetErr(errRedisNotInitialized)
	return cmd
}

func (p *redisClientProxy) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redislib.BoolCmd {
	if client := activeRedisClient(); client != nil {
		return client.SetNX(ctx, key, value, expiration)
	}
	cmd := redislib.NewBoolCmd(ctx)
	cmd.SetErr(errRedisNotInitialized)
	return cmd
}

func (p *redisClientProxy) Del(ctx context.Context, keys ...string) *redislib.IntCmd {
	if client := activeRedisClient(); client != nil {
		return client.Del(ctx, keys...)
	}
	cmd := redislib.NewIntCmd(ctx)
	cmd.SetErr(errRedisNotInitialized)
	return cmd
}

func (p *redisClientProxy) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redislib.Cmd {
	if client := activeRedisClient(); client != nil {
		return client.Eval(ctx, script, keys, args...)
	}
	cmd := redislib.NewCmd(ctx)
	cmd.SetErr(errRedisNotInitialized)
	return cmd
}

func (p *redisClientProxy) Close() error {
	RedisClose()
	return nil
}
