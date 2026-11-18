package dao

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"xops-aegis-guard/pkg/db"
	"xops-aegis-guard/server/apiserver/alert/model"
	alerttools "xops-aegis-guard/server/apiserver/alert/tools"
	jsonutil "xops-aegis-guard/util/json"

	"gorm.io/gorm"
)

const (
	occurrenceTable        = "security_alert_occurrences"
	occurrenceCompactTable = "security_alert_occurrences_compact"
	occurrenceRowTable     = "security_alert_occurrences_row_migration"
	occurrenceEventIndex   = "uk_security_alert_occurrences_alert_event"
)

// MigrateOccurrenceTable 将旧的“一次发生一行”历史表合并为“一条告警一行”。
// 迁移完成后直接删除旧结构，不再生成 legacy 备份表。
func MigrateOccurrenceTable() error {
	orm := db.GetOrm()
	var oldColumnCount int64
	if err := orm.Raw(`
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = ?
  AND column_name = 'payload'`, occurrenceTable).Scan(&oldColumnCount).Error; err != nil {
		return err
	}
	if oldColumnCount == 0 {
		return nil
	}

	createSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  id bigint unsigned NOT NULL AUTO_INCREMENT,
  alert_id bigint unsigned NOT NULL,
  occurred_at JSON NOT NULL,
  created_at datetime(3) DEFAULT NULL,
  updated_at datetime(3) DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_security_alert_occurrences_alert_id (alert_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='安全告警发生时间历史'`, occurrenceCompactTable)
	if err := orm.Exec(createSQL).Error; err != nil {
		return err
	}
	if err := orm.Exec("TRUNCATE TABLE " + occurrenceCompactTable).Error; err != nil {
		return err
	}

	insertSQL := fmt.Sprintf(`
INSERT INTO %s (alert_id, occurred_at, created_at, updated_at)
SELECT grouped_times.alert_id,
       JSON_ARRAYAGG(grouped_times.occurred_at),
       MIN(grouped_times.created_at),
       MAX(grouped_times.updated_at)
FROM (
  SELECT alert_id,
         DATE_FORMAT(COALESCE(occurred_at, captured_at), '%%Y-%%m-%%dT%%H:%%i:%%s') AS occurred_at,
         MIN(COALESCE(created_at, captured_at)) AS created_at,
         MAX(COALESCE(updated_at, captured_at)) AS updated_at
  FROM %s
  WHERE deleted_at IS NULL
  GROUP BY alert_id, DATE_FORMAT(COALESCE(occurred_at, captured_at), '%%Y-%%m-%%dT%%H:%%i:%%s')
) AS grouped_times
WHERE grouped_times.occurred_at IS NOT NULL
GROUP BY grouped_times.alert_id`, occurrenceCompactTable, occurrenceTable)
	if err := orm.Exec(insertSQL).Error; err != nil {
		return err
	}

	if err := orm.Exec("DROP TABLE " + occurrenceTable).Error; err != nil {
		return err
	}
	return orm.Exec("RENAME TABLE " + occurrenceCompactTable + " TO " + occurrenceTable).Error
}

// MigrateOccurrenceRows 将旧的“一条告警一行、时间数组存储”迁移为每次发生一行。
// 迁移保留所有数组中的时间，并把时间统一落到秒。
func MigrateOccurrenceRows() error {
	orm := db.GetOrm()
	var dataType string
	if err := orm.Raw(`
SELECT data_type
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = ?
  AND column_name = 'occurred_at'`, occurrenceTable).Scan(&dataType).Error; err != nil {
		return err
	}
	if !strings.EqualFold(dataType, "json") {
		return nil
	}

	type compactRow struct {
		AlertID    uint
		OccurredAt []byte
		CreatedAt  *time.Time
	}
	var rows []compactRow
	if err := orm.Table(occurrenceTable).Select("alert_id, occurred_at, created_at").Find(&rows).Error; err != nil {
		return err
	}

	createSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  id bigint unsigned NOT NULL AUTO_INCREMENT,
  alert_id bigint unsigned NOT NULL,
  occurred_at timestamp NOT NULL,
  created_at timestamp DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_security_alert_occurrences_alert_id (alert_id),
  KEY idx_security_alert_occurrences_occurred_at (occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='安全告警发生历史'`, occurrenceRowTable)
	if err := orm.Exec(createSQL).Error; err != nil {
		return err
	}
	if err := orm.Exec("TRUNCATE TABLE " + occurrenceRowTable).Error; err != nil {
		return err
	}

	tx := orm.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	for _, row := range rows {
		var values []string
		if err := jsonutil.Unmarshal(row.OccurredAt, &values); err != nil {
			tx.Rollback()
			return fmt.Errorf("parse occurrence times for alert %d: %w", row.AlertID, err)
		}
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			occurredAt, err := parseOccurrenceTime(value)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("parse occurrence time %q for alert %d: %w", value, row.AlertID, err)
			}
			key := occurredAt.Format("2006-01-02 15:04:05")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			createdAt := row.CreatedAt
			if createdAt == nil {
				createdAt = &occurredAt
			}
			if err := tx.Table(occurrenceRowTable).Create(map[string]any{
				"alert_id":    row.AlertID,
				"occurred_at": occurredAt,
				"created_at":  createdAt,
			}).Error; err != nil {
				tx.Rollback()
				return err
			}
		}
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}

	if err := orm.Exec("DROP TABLE " + occurrenceTable).Error; err != nil {
		return err
	}
	return orm.Exec("RENAME TABLE " + occurrenceRowTable + " TO " + occurrenceTable).Error
}

// MigrateOccurrenceEventKeys adds the idempotency key used by the collector.
// Existing history is preserved with per-row legacy keys. The latest row for
// every aggregate alert is then seeded with the same key the collector will
// generate, preventing an immediate duplicate after deployment.
func MigrateOccurrenceEventKeys() error {
	orm := db.GetOrm()
	if !orm.Migrator().HasTable(&model.AlertOccurrence{}) {
		return nil
	}
	if !orm.Migrator().HasColumn(&model.AlertOccurrence{}, "event_key") {
		if err := orm.Exec("ALTER TABLE " + occurrenceTable + " ADD COLUMN event_key BINARY(32) NULL COMMENT '云端发生事件唯一键' AFTER alert_id").Error; err != nil {
			return err
		}
	}
	if orm.Migrator().HasIndex(&model.AlertOccurrence{}, occurrenceEventIndex) {
		return nil
	}
	if err := orm.Exec(`
UPDATE security_alert_occurrences
SET event_key = UNHEX(SHA2(CONCAT('legacy:', alert_id, ':', id), 256))
WHERE event_key IS NULL OR OCTET_LENGTH(event_key) <> 32`).Error; err != nil {
		return err
	}

	var alerts []model.SecurityAlert
	if err := orm.Select("id, alert_type, provider_alert_id, fingerprint, last_provider_time, last_payload").Find(&alerts).Error; err != nil {
		return err
	}
	for i := range alerts {
		if alerts[i].LastProviderTime == nil {
			continue
		}
		payload := make(map[string]any)
		_ = jsonutil.Unmarshal([]byte(alerts[i].LastPayload), &payload)
		key := alerttools.EventKey(alerts[i].AlertType, alerts[i].Fingerprint, alerts[i].ProviderAlertID, alerts[i].LastProviderTime, payload)

		var occurrence model.AlertOccurrence
		err := orm.Where("alert_id = ? AND occurred_at = ?", alerts[i].ID, alerts[i].LastProviderTime.Truncate(time.Second)).
			Order("id DESC").First(&occurrence).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = orm.Where("alert_id = ?", alerts[i].ID).Order("occurred_at DESC, id DESC").First(&occurrence).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := orm.Model(&occurrence).Update("event_key", key).Error; err != nil {
			return err
		}
	}
	if err := orm.Exec("ALTER TABLE " + occurrenceTable + " MODIFY COLUMN event_key BINARY(32) NOT NULL COMMENT '云端发生事件唯一键'").Error; err != nil {
		return err
	}
	return orm.Exec("CREATE UNIQUE INDEX " + occurrenceEventIndex + " ON " + occurrenceTable + " (alert_id, event_key)").Error
}

func parseOccurrenceTime(value string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02T15:04:05.000000",
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed.Truncate(time.Second), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format")
}
