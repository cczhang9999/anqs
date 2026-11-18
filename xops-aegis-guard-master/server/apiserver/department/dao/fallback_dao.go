package dao

import (
	"errors"

	"xops-aegis-guard/pkg/cache"
)

const fallbackRobotKey = "xops-aegis-guard:department-fallback-robot"

type fallbackRobotConfig struct {
	RobotID int `json:"robot_id"`
}

// GetFallbackRobotID 获取未分组告警使用的默认机器人 ID。
// 未配置时返回 0，表示不发送兜底告警。
func GetFallbackRobotID() (int, error) {
	var config fallbackRobotConfig
	if err := cache.Get(fallbackRobotKey, &config); cache.IsMissing(err) || cache.IsUnavailable(err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return config.RobotID, nil
}

// SaveFallbackRobotID 保存未分组告警的默认机器人 ID。
func SaveFallbackRobotID(robotID int) error {
	if cache.Default == nil {
		return errors.New("redis is unavailable")
	}
	return cache.Set(fallbackRobotKey, fallbackRobotConfig{RobotID: robotID}, 0)
}
