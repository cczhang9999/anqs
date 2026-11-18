package dao

import (
	"errors"
	"fmt"
	"time"

	"xops-aegis-guard/pkg/db"
	"xops-aegis-guard/server/apiserver/alert/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UpsertResult struct {
	NewAlert            bool
	NewOccurrence       bool
	ContentChanged      bool
	ProviderTimeChanged bool
}

// ListOptions 是安全告警列表的内部查询参数。设备可见范围已由 service
// 独立计算，DAO 只据此查询 security_alerts 表。
type ListOptions struct {
	Limit               int
	Offset              int
	Source              string
	AlertType           string
	Status              string
	Severity            string
	Keyword             string
	KeywordDeviceIDs    []uint
	From                string
	To                  string
	DeviceIDs           []uint
	IncludeUnassociated bool
}

const alertListOrder = `COALESCE(security_alerts.last_provider_time, security_alerts.last_seen_at) DESC,
security_alerts.id DESC`

func (result UpsertResult) Changed() bool {
	return result.NewAlert || result.NewOccurrence || result.ContentChanged
}

func Upsert(alert *model.SecurityAlert, eventKey []byte) (UpsertResult, error) {
	if len(eventKey) != 32 {
		return UpsertResult{}, fmt.Errorf("invalid alert occurrence event key length: %d", len(eventKey))
	}
	var result UpsertResult
	orm := db.GetOrm()
	err := orm.Transaction(func(tx *gorm.DB) error {
		var existing model.SecurityAlert
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source = ? AND product_type = ? AND alert_type = ? AND fingerprint = ?", alert.Source, alert.ProductType, alert.AlertType, alert.Fingerprint).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			alert.OccurrenceCount = 1
			alert.PendingCount = 1
			alert.ConfirmationMigrated = true
			if err := tx.Create(alert).Error; err != nil {
				return err
			}
			inserted, err := insertOccurrence(tx, alert.ID, eventKey, alert.LastProviderTime)
			if err != nil {
				return err
			}
			if !inserted {
				return errors.New("new alert occurrence was not inserted")
			}
			result.NewAlert = true
			result.NewOccurrence = true
			result.ContentChanged = true
			result.ProviderTimeChanged = alert.LastProviderTime != nil
			return nil
		}
		if err != nil {
			return err
		}

		if !existing.ConfirmationMigrated {
			if err := migrateConfirmation(tx, &existing); err != nil {
				return err
			}
			if err := tx.First(&existing, existing.ID).Error; err != nil {
				return err
			}
		}
		alert.ID = existing.ID
		inheritIncidentDeviceID(alert, &existing)
		inserted, err := insertOccurrence(tx, existing.ID, eventKey, alert.LastProviderTime)
		if err != nil {
			return err
		}
		result.NewOccurrence = inserted

		status := "firing"
		if existing.Status == "confirmed" && !inserted {
			status = "confirmed"
		}
		updates := map[string]interface{}{
			"status":       status,
			"last_seen_at": alert.LastSeenAt,
		}
		if alert.DeviceID != 0 {
			updates["device_id"] = alert.DeviceID
		}
		if inserted {
			updates["occurrence_count"] = gorm.Expr("occurrence_count + 1")
			updates["pending_count"] = gorm.Expr("pending_count + 1")
		}

		// Delayed provider events still belong in the occurrence history, but must
		// not replace the latest alert snapshot shown by the API.
		latestSnapshot := existing.LastProviderTime == nil
		if alert.LastProviderTime != nil {
			latestSnapshot = existing.LastProviderTime == nil || !alert.LastProviderTime.Before(*existing.LastProviderTime)
			result.ProviderTimeChanged = existing.LastProviderTime == nil || alert.LastProviderTime.After(*existing.LastProviderTime)
		}
		if latestSnapshot {
			result.ContentChanged = alert.ContentHash != "" && alert.ContentHash != existing.ContentHash
			severity := higherSeverity(existing.Severity, alert.Severity)
			if alert.AlertType == "custom_incident" {
				// Cloud SIEM's message represents the latest incident snapshot, so its
				// severity must use the same version instead of the historical maximum.
				severity = alert.Severity
			}
			updates["title"] = alert.Title
			updates["severity"] = severity
			updates["last_payload"] = alert.LastPayload
			updates["last_message"] = alert.LastMessage
			updates["content_hash"] = alert.ContentHash
			if existing.FirstProviderTime == nil && alert.FirstProviderTime != nil {
				updates["first_provider_time"] = alert.FirstProviderTime
			}
			if alert.LastProviderTime != nil && (existing.LastProviderTime == nil || alert.LastProviderTime.After(*existing.LastProviderTime)) {
				updates["last_provider_time"] = alert.LastProviderTime
			}
		}
		return tx.Model(&existing).Updates(updates).Error
	})
	return result, err
}

// inheritIncidentDeviceID keeps an already routed Cloud SIEM incident on its
// assigned device when an individual provider refresh temporarily omits the
// asset relationship. Without this, the persisted alert remains associated
// but the in-memory row used for notification is routed to the fallback robot.
func inheritIncidentDeviceID(alert, existing *model.SecurityAlert) {
	if alert == nil || existing == nil || alert.AlertType != "custom_incident" || alert.DeviceID != 0 {
		return
	}
	alert.DeviceID = existing.DeviceID
}

func insertOccurrence(tx *gorm.DB, alertID uint, eventKey []byte, occurredAt *time.Time) (bool, error) {
	value := time.Now()
	if occurredAt != nil {
		value = *occurredAt
	}
	result := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "alert_id"}, {Name: "event_key"}},
		DoNothing: true,
	}).Create(&model.AlertOccurrence{
		AlertID:    alertID,
		EventKey:   eventKey,
		OccurredAt: value.Truncate(time.Second),
	})
	return result.RowsAffected == 1, result.Error
}

func IsNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

func higherSeverity(a, b string) string {
	rank := func(value string) int {
		return map[string]int{"info": 1, "low": 2, "remind": 2, "medium": 3, "suspicious": 3, "high": 4, "serious": 5}[value]
	}
	if rank(b) > rank(a) {
		return b
	}
	if a != "" {
		return a
	}
	return b
}

func List(options ListOptions) ([]model.SecurityAlert, int64, error) {
	query := applyDeviceScope(db.GetOrm().Model(&model.SecurityAlert{}), options.DeviceIDs, options.IncludeUnassociated)
	if options.Source != "" {
		query = query.Where("security_alerts.source = ?", options.Source)
	}
	if options.AlertType != "" {
		query = query.Where("security_alerts.alert_type = ?", options.AlertType)
	}
	if options.Status != "" {
		query = query.Where("security_alerts.status = ?", options.Status)
	}
	if options.Severity != "" {
		query = query.Where("security_alerts.severity = ?", options.Severity)
	}
	if options.Keyword != "" {
		like := "%" + options.Keyword + "%"
		if len(options.KeywordDeviceIDs) == 0 {
			query = query.Where("security_alerts.title LIKE ? OR security_alerts.provider_alert_id LIKE ? OR security_alerts.last_message LIKE ?", like, like, like)
		} else {
			query = query.Where("security_alerts.title LIKE ? OR security_alerts.provider_alert_id LIKE ? OR security_alerts.last_message LIKE ? OR security_alerts.device_id IN ?", like, like, like, options.KeywordDeviceIDs)
		}
	}
	if options.From != "" {
		query = query.Where("COALESCE(security_alerts.last_provider_time, security_alerts.last_seen_at) >= ?", options.From+" 00:00:00")
	}
	if options.To != "" {
		if end, err := time.ParseInLocation("2006-01-02", options.To, time.Local); err == nil {
			query = query.Where("COALESCE(security_alerts.last_provider_time, security_alerts.last_seen_at) < ?", end.AddDate(0, 0, 1))
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if options.Limit <= 0 || options.Limit > 200 {
		options.Limit = 50
	}
	var rows []model.SecurityAlert
	err := query.Order(alertListOrder).Limit(options.Limit).Offset(options.Offset).Find(&rows).Error
	return rows, total, err
}

func All(deviceIDs []uint, includeUnassociated bool) ([]model.SecurityAlert, error) {
	var rows []model.SecurityAlert
	err := applyDeviceScope(db.GetOrm().Model(&model.SecurityAlert{}), deviceIDs, includeUnassociated).
		Order(alertListOrder).Find(&rows).Error
	return rows, err
}

func Stats(todayStart, tomorrowStart time.Time, deviceIDs []uint, includeUnassociated bool) (total, pending, confirmed, today int64, severity map[string]int64, err error) {
	scoped := func() *gorm.DB {
		query := db.GetOrm().Session(&gorm.Session{NewDB: true}).Model(&model.SecurityAlert{})
		return applyDeviceScope(query, deviceIDs, includeUnassociated)
	}
	if err = scoped().Count(&total).Error; err != nil {
		return
	}
	if err = scoped().Where("security_alerts.status <> ?", "confirmed").Count(&pending).Error; err != nil {
		return
	}
	if err = scoped().Where("security_alerts.status = ?", "confirmed").Count(&confirmed).Error; err != nil {
		return
	}
	if err = scoped().Where("COALESCE(security_alerts.last_provider_time, security_alerts.last_seen_at) >= ? AND COALESCE(security_alerts.last_provider_time, security_alerts.last_seen_at) < ?", todayStart, tomorrowStart).Count(&today).Error; err != nil {
		return
	}
	severity = map[string]int64{}
	rows := make([]struct {
		Severity string `gorm:"column:severity"`
		Count    int64  `gorm:"column:count"`
	}, 0)
	if err = scoped().Select("security_alerts.severity, COUNT(*) AS count").Group("security_alerts.severity").Scan(&rows).Error; err != nil {
		return
	}
	for _, row := range rows {
		severity[row.Severity] = row.Count
	}
	return
}

func Get(id uint) (*model.SecurityAlert, error) {
	var row model.SecurityAlert
	err := db.GetOrm().First(&row, id).Error
	return &row, err
}

func applyDeviceScope(query *gorm.DB, deviceIDs []uint, includeUnassociated bool) *gorm.DB {
	if len(deviceIDs) == 0 {
		if includeUnassociated {
			return query.Where("security_alerts.device_id = 0")
		}
		return query.Where("1 = 0")
	}
	if includeUnassociated {
		return query.Where("security_alerts.device_id = 0 OR security_alerts.device_id IN ?", deviceIDs)
	}
	return query.Where("security_alerts.device_id IN ?", deviceIDs)
}

func GetByIdentity(source, productType, alertType, fingerprint string) (*model.SecurityAlert, error) {
	var row model.SecurityAlert
	err := db.GetOrm().Where("source = ? AND product_type = ? AND alert_type = ? AND fingerprint = ?", source, productType, alertType, fingerprint).First(&row).Error
	return &row, err
}

func ListOccurrences(alertID uint) ([]model.AlertOccurrence, error) {
	var rows []model.AlertOccurrence
	err := db.GetOrm().Where("alert_id = ?", alertID).Order("occurred_at DESC, id DESC").Find(&rows).Error
	return rows, err
}

func ListUnassociated(limit int) ([]model.SecurityAlert, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var rows []model.SecurityAlert
	err := db.GetOrm().Where("device_id = ?", 0).Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func BindDevice(id, deviceID uint) error {
	return db.GetOrm().Model(&model.SecurityAlert{}).Where("id = ? AND device_id = ?", id, 0).Updates(map[string]interface{}{
		"device_id": deviceID,
	}).Error
}

// ListSparseMessages returns alert snapshots that may need their stored payload
// or notification text rebuilt. The service performs the final structural and
// desensitization checks before writing anything.
func ListSparseMessages(limit int) ([]model.SecurityAlert, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var rows []model.SecurityAlert
	err := db.GetOrm().
		Where(`
			(alert_type IN ? AND CHAR_LENGTH(last_message) <= ?)
			OR alert_type = ?
			OR last_message LIKE ?
			OR (severity <> '' AND last_message NOT LIKE ?)
			OR (severity IN ('serious','high','suspicious','medium','low','info','unknown')
				AND last_message NOT LIKE CONCAT('%告警等级：', CASE severity
					WHEN 'serious' THEN '严重'
					WHEN 'high' THEN '高危'
					WHEN 'suspicious' THEN '可疑'
					WHEN 'medium' THEN '中危'
					WHEN 'low' THEN '低危'
					WHEN 'info' THEN '提示'
					WHEN 'unknown' THEN '未标识'
				END, '%'))
			OR TRIM(last_message) LIKE ?
			OR INSTR(last_message, CHAR(13)) > 0
			OR last_message LIKE ?
			OR last_message LIKE ?
			OR (alert_type = ? AND (last_message LIKE ? OR last_message LIKE ? OR last_message LIKE ?))`,
			[]string{"honeypot", "custom_incident"}, 256,
			"suspicious_event",
			"%告警详情：{%",
			"%告警等级：%",
			"**%",
			"%mysqldump%",
			"%mysqlpump%",
			"core_file", "%告警id：%e+%", "%告警首次时间：%e+%", "%告警末次时间：%e+%").
		Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func UpdateSnapshot(id, deviceID uint, payload, message, contentHash string) error {
	updates := map[string]interface{}{
		"last_payload": payload,
		"last_message": message,
		"content_hash": contentHash,
	}
	if deviceID != 0 {
		updates["device_id"] = deviceID
	}
	return db.GetOrm().Model(&model.SecurityAlert{}).Where("id = ?", id).Updates(updates).Error
}

func LatestProviderTime(productType string) (*time.Time, error) {
	var row model.SecurityAlert
	query := db.GetOrm().Where("product_type = ? AND last_provider_time IS NOT NULL", productType).Order("last_provider_time DESC").Limit(1)
	if err := query.Find(&row).Error; err != nil {
		return nil, err
	}
	if row.LastProviderTime == nil {
		return nil, nil
	}
	value := *row.LastProviderTime
	return &value, nil
}

func Save(row *model.SecurityAlert) error {
	return db.GetOrm().Transaction(func(tx *gorm.DB) error {
		if row.ID != 0 {
			var existing model.SecurityAlert
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, row.ID).Error; err != nil {
				return err
			}
			// Snapshot edits cannot replace confirmation state, including confirmations
			// submitted after the service initially read this alert.
			row.ConfirmationMigrated = existing.ConfirmationMigrated
			row.ConfirmationCount = existing.ConfirmationCount
			row.ConfirmedCount = existing.ConfirmedCount
			row.PendingCount = existing.PendingCount
			row.ConfirmedAt = existing.ConfirmedAt
			row.ConfirmedBy = existing.ConfirmedBy
			row.ConfirmedByID = existing.ConfirmedByID
			row.ConfirmNote = existing.ConfirmNote
			row.Status = existing.Status
			row.OccurrenceCount = existing.OccurrenceCount
		}
		if err := tx.Save(row).Error; err != nil {
			return err
		}
		if err := migrateConfirmation(tx, row); err != nil {
			return err
		}
		return tx.First(row, row.ID).Error
	})
}

// Confirm remains available to internal callers and shares the same transactional path.
func Confirm(id uint, note string, confirmedByID int, confirmedBy string, confirmedAt time.Time) error {
	_, err := ConfirmOccurrences([]uint{id}, 0, note, confirmedByID, confirmedBy, confirmedAt)
	return err
}

func Touch(before time.Time) error {
	return db.GetOrm().Model(&model.SecurityAlert{}).Where("status = ? AND last_seen_at < ?", "firing", before).Update("status", "stale").Error
}
