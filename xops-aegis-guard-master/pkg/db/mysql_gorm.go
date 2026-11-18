package db

import (
	"database/sql"
	"log"
	"time"

	"xops-aegis-guard/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var orm *gorm.DB

// InitDB initializes the process-wide database handle, matching the
// xops-port-check access pattern used by DAO packages.
func InitDB(conf config.Database) error {
	level := logger.Silent
	if conf.Debug {
		level = logger.Info
	}
	database, err := gorm.Open(mysql.Open(conf.Connection), &gorm.Config{Logger: logger.New(log.New(log.Writer(), "", log.LstdFlags), logger.Config{LogLevel: level, Colorful: conf.Colorful})})
	if err != nil {
		return err
	}
	sqlDB, err := database.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(conf.MaxOpen)
	sqlDB.SetMaxIdleConns(conf.MaxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(conf.ConnMaxLifeTime) * time.Second)
	orm = database
	return nil
}

func GetOrm() *gorm.DB { return orm }

func SQL() (*sql.DB, error) { return orm.DB() }
