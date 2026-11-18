package task

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"xops-aegis-guard/logger"
	"xops-aegis-guard/pkg/redisstore"
	alertservice "xops-aegis-guard/server/apiserver/alert/service"
)

const (
	alertSchedule  = "0 * * * * *"   // 每分钟第 0 秒
	deviceSchedule = "0 */5 * * * *" // 每 5 分钟第 0 秒
)

// StartJob starts all in-process scheduled jobs, matching the
// xops-waf-new/server/task pattern. It does not depend on system crontab.
func StartJob() *cron.Cron {
	scheduler := cron.New(cron.WithSeconds())
	if _, err := scheduler.AddFunc(alertSchedule, collectAlerts); err != nil {
		logger.Errorf("register alert task failed: %v", err)
	}
	if _, err := scheduler.AddFunc(deviceSchedule, syncDevices); err != nil {
		logger.Errorf("register device task failed: %v", err)
	}
	scheduler.Start()
	logger.Infof("internal task scheduler started: alert_schedule=%s device_schedule=%s", alertSchedule, deviceSchedule)
	return scheduler
}

func collectAlerts() {
	logger.Info("alert task start")
	if err := alertservice.CollectNow(); err != nil {
		logger.Errorf("collect aliyun alerts failed: %v", err)
	}
	logger.Info("alert task end")
}

func syncDevices() {
	logger.Info("device sync task start")
	if err := RunDeviceSync(); err != nil {
		logger.Errorf("device sync task failed: %v", err)
	}
	logger.Info("device sync task end")
}

// RunDeviceSync executes one complete device synchronization cycle. It is
// shared by the startup path and the periodic task so both paths use the same
// Redis lock and alert rebinding behavior.
func RunDeviceSync() error {
	store := redisstore.New()
	release, err := store.AcquireNamedLock("device-sync", 10*time.Second)
	if err != nil {
		return fmt.Errorf("acquire device sync lock: %w", err)
	}
	defer release()
	if err := alertservice.SyncDevices(); err != nil {
		return fmt.Errorf("sync aliyun devices: %w", err)
	}
	if err := alertservice.RebindUnassociated(); err != nil {
		return fmt.Errorf("rebind unassociated alerts: %w", err)
	}
	if err := alertservice.RefreshSparseMessages(); err != nil {
		return fmt.Errorf("refresh sparse alert messages: %w", err)
	}
	return nil
}
