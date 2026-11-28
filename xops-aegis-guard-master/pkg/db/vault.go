package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"xops-aegis-guard/config"
	"xops-aegis-guard/logger"
)

type vaultCredentialFile struct {
	LegalTenderDB struct {
		Connection string `yaml:"connection"`
	} `yaml:"legal_tender_db"`
	VaultCfg struct {
		LeaseID       string `yaml:"lease_id"`
		LeaseDuration int    `yaml:"lease_duration"`
	} `yaml:"vault_cfg"`
}

type vaultDBWatcher struct {
	filePath          string
	refreshInterval   time.Duration
	drainWarning      time.Duration
	minimumDrain      time.Duration
	poolConfig        config.Database
	currentLeaseID    string
	currentConnection string
	cancel            context.CancelFunc
	done              chan struct{}
	drainWG           sync.WaitGroup
	reloadMu          sync.Mutex
}

var (
	vaultDBWatcherMu sync.Mutex
	dbVaultWatcher   *vaultDBWatcher
)

func initVaultDB(conf config.Database) error {
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

	stopVaultDBWatcher()
	ctx, cancel := context.WithCancel(context.Background())
	watcher := &vaultDBWatcher{
		filePath:        strings.TrimSpace(conf.VaultCredentialsFile),
		refreshInterval: refreshInterval,
		drainWarning:    drainWarning,
		minimumDrain:    minimumDrain,
		poolConfig:      conf,
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
			return fmt.Errorf("load initial Vault database credentials: %w", err)
		}
		logger.Warnf("[Vault] waiting for initial database credentials: %v", err)
		time.Sleep(time.Second)
	}

	vaultDBWatcherMu.Lock()
	dbVaultWatcher = watcher
	vaultDBWatcherMu.Unlock()
	go watcher.run(ctx)
	return nil
}

func (w *vaultDBWatcher) run(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(w.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.reload(false); err != nil {
				logger.Errorf("[Vault] apply database credentials failed; current pool remains active: %v", err)
			}
		}
	}
}

func (w *vaultDBWatcher) reload(initial bool) error {
	w.reloadMu.Lock()
	defer w.reloadMu.Unlock()

	credentials, err := readVaultCredentialFile(w.filePath)
	if err != nil {
		return err
	}
	connection := credentials.LegalTenderDB.Connection
	if !initial && credentials.VaultCfg.LeaseID == w.currentLeaseID && connection == w.currentConnection {
		return nil
	}

	next, err := openDB(connection, w.poolConfig)
	if err != nil {
		return fmt.Errorf("create candidate GORM pool: %w", err)
	}
	nextSQL, err := next.DB()
	if err != nil {
		return fmt.Errorf("get candidate sql.DB: %w", err)
	}
	verifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = nextSQL.PingContext(verifyCtx); err != nil {
		_ = nextSQL.Close()
		return fmt.Errorf("ping candidate database pool: %w", err)
	}
	var mysqlUser, databaseName sql.NullString
	if err = nextSQL.QueryRowContext(verifyCtx, "SELECT CURRENT_USER(), DATABASE()").Scan(&mysqlUser, &databaseName); err != nil {
		_ = nextSQL.Close()
		return fmt.Errorf("verify candidate database identity: %w", err)
	}
	nextDSN, err := mysqldriver.ParseDSN(connection)
	if err != nil {
		_ = nextSQL.Close()
		return fmt.Errorf("parse candidate MySQL DSN: %w", err)
	}
	if nextDSN.DBName != "" && databaseName.String != nextDSN.DBName {
		_ = nextSQL.Close()
		return fmt.Errorf("candidate database mismatch: expected %q, got %q", nextDSN.DBName, databaseName.String)
	}

	oldLeaseID := w.currentLeaseID
	oldDSN, _ := mysqldriver.ParseDSN(w.currentConnection)
	oldUser := ""
	if oldDSN != nil {
		oldUser = oldDSN.User
	}
	old := swapDB(next)
	w.currentLeaseID = credentials.VaultCfg.LeaseID
	w.currentConnection = connection

	if initial {
		logger.Warnf(
			"[Vault] initial MySQL pool ready: user=%q database=%q lease_id=%q lease_duration=%ds",
			mysqlUser.String,
			databaseName.String,
			credentials.VaultCfg.LeaseID,
			credentials.VaultCfg.LeaseDuration,
		)
	} else {
		logger.Warnf(
			"[Vault] new database credentials verified and switched: lease_id=%q user=%q database=%q",
			credentials.VaultCfg.LeaseID,
			nextDSN.User,
			databaseName.String,
		)
		logger.Warnf(
			"[Vault] old database credentials entering pool drain: lease_id=%q user=%q",
			oldLeaseID,
			oldUser,
		)
		logger.Warnf("[Vault] new database credential lease: %d seconds", credentials.VaultCfg.LeaseDuration)
	}
	if old != nil {
		w.drainWG.Add(1)
		go w.drainOldDB(old, oldLeaseID, oldUser)
	}
	return nil
}

func (w *vaultDBWatcher) drainOldDB(old *gorm.DB, leaseID, username string) {
	defer w.drainWG.Done()
	oldSQL, err := old.DB()
	if err != nil {
		logger.Errorf("[Vault] get old sql.DB failed: lease_id=%q user=%q err=%v", leaseID, username, err)
		return
	}
	defer func() {
		if err := oldSQL.Close(); err != nil {
			logger.Errorf("[Vault] close old MySQL pool failed: lease_id=%q user=%q err=%v", leaseID, username, err)
			return
		}
		logger.Warnf("[Vault] old MySQL pool closed: lease_id=%q user=%q", leaseID, username)
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
	for oldSQL.Stats().InUse > 0 {
		if !warned && time.Since(startedAt) >= w.drainWarning {
			warned = true
			logger.Warnf(
				"[Vault] old MySQL pool is still in use; waiting without interruption: lease_id=%q user=%q in_use=%d",
				leaseID,
				username,
				oldSQL.Stats().InUse,
			)
		}
		select {
		case <-w.done:
			return
		case <-ticker.C:
		}
	}
}

func readVaultCredentialFile(path string) (*vaultCredentialFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Vault credentials file %s: %w", path, err)
	}
	credentials := &vaultCredentialFile{}
	if err = yaml.Unmarshal(data, credentials); err != nil {
		return nil, fmt.Errorf("decode Vault credentials file %s: %w", path, err)
	}
	credentials.LegalTenderDB.Connection = strings.TrimSpace(credentials.LegalTenderDB.Connection)
	credentials.VaultCfg.LeaseID = strings.TrimSpace(credentials.VaultCfg.LeaseID)
	if credentials.LegalTenderDB.Connection == "" {
		return nil, fmt.Errorf("Vault credentials file %s is missing legal_tender_db.connection", path)
	}
	if credentials.VaultCfg.LeaseID == "" {
		return nil, fmt.Errorf("Vault credentials file %s is missing vault_cfg.lease_id", path)
	}
	return credentials, nil
}

func stopVaultDBWatcher() {
	vaultDBWatcherMu.Lock()
	watcher := dbVaultWatcher
	dbVaultWatcher = nil
	vaultDBWatcherMu.Unlock()
	if watcher == nil {
		return
	}
	watcher.cancel()
	<-watcher.done
	watcher.drainWG.Wait()
}