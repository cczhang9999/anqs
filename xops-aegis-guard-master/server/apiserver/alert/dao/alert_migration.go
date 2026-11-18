package dao

import (
	"strings"

	"xops-aegis-guard/pkg/db"
	"xops-aegis-guard/server/apiserver/alert/model"
	alerttools "xops-aegis-guard/server/apiserver/alert/tools"
	jsonutil "xops-aegis-guard/util/json"
)

// MigrateAlertTable removes duplicated snapshot columns. Department data is
// resolved from the alert's device, and created_at is the first received time.
// The raw payload remains available to the detail endpoint.
func MigrateAlertTable() error {
	orm := db.GetOrm()
	migrator := orm.Migrator()
	table := &model.SecurityAlert{}
	for _, column := range []string{"department_id", "department_snapshot", "first_seen_at", "deleted_at", "content_summary"} {
		if migrator.HasColumn(table, column) {
			if err := migrator.DropColumn(table, column); err != nil {
				return err
			}
		}
	}
	// 每个阿里云接口使用独立的等级枚举。按原始 payload 重新归一化，
	// 避免历史版本共用数字映射后造成等级倒置或显示为 unknown。
	statements := []string{`
		UPDATE security_alerts
		SET severity = CASE LOWER(JSON_UNQUOTE(JSON_EXTRACT(IF(JSON_VALID(last_payload), last_payload, '{}'), '$.Level')))
			WHEN 'serious' THEN 'serious'
			WHEN 'suspicious' THEN 'suspicious'
			WHEN 'remind' THEN 'info'
			ELSE severity
		END
		WHERE alert_type = 'suspicious_event'
		  AND JSON_VALID(last_payload)
	`, `
		UPDATE security_alerts
		SET severity = CASE JSON_UNQUOTE(JSON_EXTRACT(IF(JSON_VALID(last_payload), last_payload, '{}'), '$.RiskLevel'))
			WHEN '4' THEN 'high'
			WHEN '3' THEN 'medium'
			WHEN '2' THEN 'low'
			ELSE severity
		END
		WHERE alert_type = 'honeypot'
		  AND JSON_VALID(last_payload)
	`, `
		UPDATE security_alerts
		SET severity = CASE JSON_UNQUOTE(JSON_EXTRACT(IF(JSON_VALID(last_payload), last_payload, '{}'), '$.AlertLevel'))
			WHEN '1' THEN 'serious'
			WHEN '2' THEN 'suspicious'
			WHEN '3' THEN 'low'
			WHEN '4' THEN 'info'
			ELSE severity
		END
		WHERE alert_type = 'core_file'
		  AND JSON_VALID(last_payload)
	`, `
		UPDATE security_alerts
		SET severity = CASE JSON_UNQUOTE(JSON_EXTRACT(IF(JSON_VALID(last_payload), last_payload, '{}'), '$.ThreatLevel'))
			WHEN '5' THEN 'serious'
			WHEN '4' THEN 'high'
			WHEN '3' THEN 'medium'
			WHEN '2' THEN 'low'
			WHEN '1' THEN 'info'
			ELSE severity
		END
		WHERE alert_type = 'custom_incident'
		  AND JSON_VALID(last_payload)
	`}
	for _, statement := range statements {
		if err := orm.Exec(statement).Error; err != nil {
			return err
		}
	}
	if err := backfillAlertMetadata(); err != nil {
		return err
	}
	// Historical versions could leave confirmation metadata and status out of
	// sync. Confirmation metadata is stronger evidence than a later collector
	// refresh, so keep those rows confirmed and fill missing legacy metadata.
	if err := orm.Exec(`
		UPDATE security_alerts
		SET status = 'confirmed'
		WHERE status <> 'confirmed' AND confirmed_at IS NOT NULL AND confirmation_migrated = false
	`).Error; err != nil {
		return err
	}
	if err := orm.Exec(`
		UPDATE security_alerts
		SET confirmed_by = '历史确认'
		WHERE status = 'confirmed' AND COALESCE(confirmed_by, '') = ''
	`).Error; err != nil {
		return err
	}
	if err := orm.Exec(`
		UPDATE security_alerts
		SET confirmed_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP)
		WHERE status = 'confirmed' AND confirmed_at IS NULL
	`).Error; err != nil {
		return err
	}
	return nil
}

func backfillAlertMetadata() error {
	orm := db.GetOrm()
	var rows []model.SecurityAlert
	if err := orm.Select("id, alert_type, last_payload, last_message, first_provider_time, last_provider_time, created_at, content_hash").Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		updates := map[string]any{}
		expectedHash := alerttools.ContentHash(rows[i].LastMessage)
		if strings.TrimSpace(rows[i].ContentHash) != expectedHash {
			updates["content_hash"] = expectedHash
		}
		if rows[i].FirstProviderTime == nil {
			payload := map[string]any{}
			_ = jsonutil.Unmarshal([]byte(rows[i].LastPayload), &payload)
			fallback := rows[i].LastProviderTime
			if fallback == nil {
				fallback = rows[i].CreatedAt
			}
			if first := alerttools.FirstProviderTime(rows[i].AlertType, payload, fallback); first != nil {
				updates["first_provider_time"] = first
			}
		}
		if len(updates) > 0 {
			if err := orm.Model(&model.SecurityAlert{}).Where("id = ?", rows[i].ID).UpdateColumns(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
