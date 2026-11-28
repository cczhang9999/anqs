package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	redislib "github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"

	"xops-aegis-guard/config"
	"xops-aegis-guard/logger"
)

type vaultRedisCredentialFile struct {
	Redis struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"redis"`
	VaultRedisCfg struct {
		LeaseID       string `yaml:"lease_id"`
		LeaseDuration int    `yaml:"lease_duration"`
	} `yaml:"vault_redis_cfg"`
}

type vaultRedisWatcher struct {
	filePath        string
	refreshInterval time.Duration
	drainWarning    time.Duration
	minimumDrain    time.Duration
	clientConfig    config.Redis
	currentLeaseID  string
	currentUsername string
	currentPassword string
	cancel          context.CancelFunc
	done            chan struct{}
	drainWG         sync.WaitGroup
	reloadMu        sync.Mutex
}

var (
	vaultRedisWatcherMu sync.Mutex
	redisVaultWatcher   *vaultRedisWatcher
)

func initVaultRedis(conf config.Redis) error {
	refreshInterval := time.Duration(conf.VaultRefreshSeconds) * time.Second
	if refreshInterval <= 0 {
		refreshInterval = 2 * time.Second
	}
	drainWarning := time.Duration(conf.VaultDrainSeconds) * time.Second
	if drainWarning <= 0 {
		drainWarning = 30 * time.Second
	}
	minimumDrain := time.Duration(conf.VaultMinimumDrainSeconds) * time.Second
	if minimumDrain <= 0 {
		minimumDrain = 2 * time.Second
	}
	initialWait := time.Duration(conf.VaultInitialWaitSeconds) * time.Second
	if initialWait <= 0 {
		initialWait = 60 * time.Second
	}

	stopVaultRedisWatcher()
	ctx, cancel := context.WithCancel(context.Background())
	watcher := &vaultRedisWatcher{
		filePath:        strings.TrimSpace(conf.VaultCredentialsFile),
		refreshInterval: refreshInterval,
		drainWarning:    drainWarning,
		minimumDrain:    minimumDrain,
		clientConfig:    conf,
		cancel:          cancel,
		done:            make(chan struct{}),
	}
	deadline := time.Now().Add(initialWait)
	for {
		err := watcher.reload(true)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			return fmt.Errorf("load initial Vault Redis credentials: %w", err)
		}
		logger.Warnf("[Vault] waiting for initial Redis credentials: %v", err)
		time.Sleep(time.Second)
	}

	vaultRedisWatcherMu.Lock()
	redisVaultWatcher = watcher
	vaultRedisWatcherMu.Unlock()
	go watcher.run(ctx)
	return nil
}

func (w *vaultRedisWatcher) run(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(w.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.reload(false); err != nil {
				logger.Errorf("[Vault] apply Redis credentials failed; current client remains active: %v", err)
			}
		}
	}
}

func (w *vaultRedisWatcher) reload(initial bool) error {
	w.reloadMu.Lock()
	defer w.reloadMu.Unlock()

	credentials, err := readVaultRedisCredentialFile(w.filePath)
	if err != nil {
		return err
	}
	if !initial &&
		credentials.VaultRedisCfg.LeaseID == w.currentLeaseID &&
		credentials.Redis.Username == w.currentUsername &&
		credentials.Redis.Password == w.currentPassword {
		return nil
	}

	nextConfig := w.clientConfig
	nextConfig.Username = credentials.Redis.Username
	nextConfig.Password = credentials.Redis.Password
	next, err := newRedisCommandClient(&nextConfig)
	if err != nil {
		return fmt.Errorf("create and verify candidate Redis client: %w", err)
	}
	oldLeaseID := w.currentLeaseID
	oldUsername := w.currentUsername
	old := swapRedisClient(next)
	w.currentLeaseID = credentials.VaultRedisCfg.LeaseID
	w.currentUsername = credentials.Redis.Username
	w.currentPassword = credentials.Redis.Password

	if initial {
		logger.Warnf(
			"[Vault] initial Redis client ready: user=%q lease_id=%q lease_duration=%ds",
			credentials.Redis.Username,
			credentials.VaultRedisCfg.LeaseID,
			credentials.VaultRedisCfg.LeaseDuration,
		)
	} else {
		logger.Warnf(
			"[Vault] new Redis credentials verified and switched: lease_id=%q user=%q",
			credentials.VaultRedisCfg.LeaseID,
			credentials.Redis.Username,
		)
		logger.Warnf(
			"[Vault] old Redis credentials entering pool drain: lease_id=%q user=%q",
			oldLeaseID,
			oldUsername,
		)
		logger.Warnf("[Vault] new Redis credential lease: %d seconds", credentials.VaultRedisCfg.LeaseDuration)
	}
	if old != nil {
		w.drainWG.Add(1)
		go w.drainOldRedisClient(old, oldLeaseID, oldUsername)
	}
	return nil
}

func (w *vaultRedisWatcher) drainOldRedisClient(client RedisCommandClient, leaseID, username string) {
	defer w.drainWG.Done()
	defer func() {
		if err := client.Close(); err != nil {
			logger.Errorf("[Vault] close old Redis pool failed: lease_id=%q user=%q err=%v", leaseID, username, err)
			return
		}
		logger.Warnf("[Vault] old Redis pool closed: lease_id=%q user=%q", leaseID, username)
	}()

	startedAt := time.Now()
	minimumTimer := time.NewTimer(w.minimumDrain)
	defer minimumTimer.Stop()
	select {
	case <-w.done:
		return
	case <-minimumTimer.C:
	}
	warned := false
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for redisClientInUse(client) > 0 {
		if !warned && time.Since(startedAt) >= w.drainWarning {
			warned = true
			logger.Warnf(
				"[Vault] old Redis pool is still in use; waiting without interruption: lease_id=%q user=%q in_use=%d",
				leaseID,
				username,
				redisClientInUse(client),
			)
		}
		select {
		case <-w.done:
			return
		case <-ticker.C:
		}
	}
}

func redisClientInUse(client RedisCommandClient) int {
	provider, ok := client.(interface {
		PoolStats() *redislib.PoolStats
	})
	if !ok {
		return 0
	}
	stats := provider.PoolStats()
	if stats == nil || stats.TotalConns <= stats.IdleConns {
		return 0
	}
	return int(stats.TotalConns - stats.IdleConns)
}

func readVaultRedisCredentialFile(path string) (*vaultRedisCredentialFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Vault Redis credentials file %s: %w", path, err)
	}
	credentials := &vaultRedisCredentialFile{}
	if err = yaml.Unmarshal(data, credentials); err != nil {
		return nil, fmt.Errorf("decode Vault Redis credentials file %s: %w", path, err)
	}
	credentials.Redis.Username = strings.TrimSpace(credentials.Redis.Username)
	credentials.VaultRedisCfg.LeaseID = strings.TrimSpace(credentials.VaultRedisCfg.LeaseID)
	if credentials.Redis.Username == "" {
		return nil, fmt.Errorf("Vault Redis credentials file %s is missing redis.username", path)
	}
	if credentials.Redis.Password == "" {
		return nil, fmt.Errorf("Vault Redis credentials file %s is missing redis.password", path)
	}
	if credentials.VaultRedisCfg.LeaseID == "" {
		return nil, fmt.Errorf("Vault Redis credentials file %s is missing vault_redis_cfg.lease_id", path)
	}
	return credentials, nil
}

func stopVaultRedisWatcher() {
	vaultRedisWatcherMu.Lock()
	watcher := redisVaultWatcher
	redisVaultWatcher = nil
	vaultRedisWatcherMu.Unlock()
	if watcher == nil {
		return
	}
	watcher.cancel()
	<-watcher.done
	watcher.drainWG.Wait()
}