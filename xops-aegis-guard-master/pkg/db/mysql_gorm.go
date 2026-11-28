package db

import (
	"database/sql"
	"fmt"
   	"log"
   	"os"
   	"strconv"
   	"strings"
    "sync"
    "time"

	"xops-aegis-guard/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ormMu sync.RWMutex
	orm   *gorm.DB
)

// InitDB initializes the process-wide database handle, matching the
// xops-port-check access pattern used by DAO packages.
func InitDB(conf config.Database) error {
	if VaultEnabled(conf) {
		return initVaultDB(conf)
	}
	next, err := openDB(conf.Connection, conf)
	if err != nil {
		return err
	}
	old := swapDB(next)
	if old != nil {
		if oldSQL, oldErr := old.DB(); oldErr == nil {
			_ = oldSQL.Close()
		}
	}
	return nil
}
// VaultEnabled reports whether MySQL credentials come from the Agent file.
func VaultEnabled(conf config.Database) bool {
	if strings.TrimSpace(conf.VaultCredentialsFile) == "" {
		return false
	}
	if raw := strings.TrimSpace(os.Getenv("VAULT_ENABLED")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		return err != nil || enabled
	}
	return true
}

func openDB(connection string, conf config.Database) (*gorm.DB, error) {
	connection = strings.TrimSpace(connection)
	if connection == "" {
		return nil, fmt.Errorf("database connection is empty")
	}
	level := logger.Silent
	if conf.Debug {
		level = logger.Info
	}
	database, err := gorm.Open(mysql.Open(connection), &gorm.Config{Logger: logger.New(log.New(log.Writer(), "", log.LstdFlags), logger.Config{LogLevel: level, Colorful: conf.Colorful})})
	if err != nil {
		return nil, err
	}
	sqlDB, err := database.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(conf.MaxOpen)
	sqlDB.SetMaxIdleConns(conf.MaxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(conf.ConnMaxLifeTime) * time.Second)
	return database, nil
}

func swapDB(next *gorm.DB) *gorm.DB {
	ormMu.Lock()
	old := orm
	orm = next
	ormMu.Unlock()
	return old
}

// GetOrm returns the currently active GORM pool. Callers must not retain it.
func GetOrm() *gorm.DB {
	ormMu.RLock()
	current := orm
	ormMu.RUnlock()
	return current
}

func SQL() (*sql.DB, error) {
	current := GetOrm()
	if current == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	return current.DB()
}

func CloseDB() {
	stopVaultDBWatcher()
	current := swapDB(nil)
	if current != nil {
		if sqlDB, err := current.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}
