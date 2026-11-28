package main

import (
	"flag"
	"os"

	"xops-aegis-guard/config"
	"xops-aegis-guard/desensitize"
	"xops-aegis-guard/logger"
	"xops-aegis-guard/logger/zaplog"
	"xops-aegis-guard/pkg/aliyun"
	"xops-aegis-guard/pkg/cache"
	rediscache "xops-aegis-guard/pkg/cache/redis"
	"xops-aegis-guard/pkg/db"
	"xops-aegis-guard/pkg/validator"
	robotapi "xops-aegis-guard/server/api"
	alertdao "xops-aegis-guard/server/apiserver/alert/dao"
	alertmodel "xops-aegis-guard/server/apiserver/alert/model"
	alertservice "xops-aegis-guard/server/apiserver/alert/service"
	"xops-aegis-guard/server/apiserver/base/router"
	departmentmodel "xops-aegis-guard/server/apiserver/department/model"
	devicedao "xops-aegis-guard/server/apiserver/device/dao"
	devicemodel "xops-aegis-guard/server/apiserver/device/model"
	rulemodel "xops-aegis-guard/server/apiserver/grouprule/model"
	"xops-aegis-guard/server/task"
)

func main() {
	configPath := flag.String("f", "./config/config.yml", "config file")
	flag.Parse()
	conf, err := config.Load(*configPath)
	if err != nil {
		panic(err)
	}
	logger.SetLogger(zaplog.InitLogger("xops_aegis_guard", conf.Logger))

	if err := db.InitDB(conf.Database); err != nil {
		logger.Errorf("open database failed: %v", err)
		os.Exit(1)
	}
	defer db.CloseDB()
	if err := db.InitRedis(conf.Redis); err != nil {
		logger.Errorf("open redis failed: %v", err)
		os.Exit(1)
	}
	defer db.RedisClose()
	cache.Default = rediscache.NewCache(db.RedisUniversalClient())
    if db.VaultEnabled(conf.Database) {
    	logger.Info("[Vault] skip database migrations for dynamic credentials")
   	} else {
		initModel()
	}
	masker := desensitize.NewMasker(conf.Desensitize)
	clients, err := aliyun.NewClients(conf.Aliyun)
	if err != nil {
		logger.Errorf("create aliyun clients failed: %v", err)
		os.Exit(1)
	}
	validator.InitValidate()
	alertservice.Init(clients, masker, robotapi.NewNotifier(conf.Robot))
	scheduler := task.StartJob()
	defer scheduler.Stop()

	logger.Infof("xops-aegis-guard started: addr=%s scheduler=internal-cron robot_enabled=%t", conf.Server.HTTPPort, conf.Robot.Enabled)
	if err := router.InitRouter(conf); err != nil {
		logger.Errorf("server stopped: %v", err)
		os.Exit(1)
	}
}

// initModel automatically creates or updates all application tables on startup,
// matching the initialization style of xops-port-check.
func initModel() error {
	if err := alertdao.MigrateOccurrenceTable(); err != nil {
		return err
	}
	if err := alertdao.MigrateOccurrenceRows(); err != nil {
		return err
	}
	if err := db.GetOrm().AutoMigrate(
		&departmentmodel.Department{},
		&devicemodel.Device{},
		&alertmodel.SecurityAlert{},
		&alertmodel.AlertOccurrence{},
		&rulemodel.GroupRule{},
	); err != nil {
		return err
	}
	if err := alertdao.MigrateOccurrenceEventKeys(); err != nil {
		return err
	}
	if err := devicedao.MigrateDeviceTable(); err != nil {
		return err
	}
	if err := alertdao.MigrateAlertTable(); err != nil {
		return err
	}
	return alertdao.MigrateConfirmations()
}
