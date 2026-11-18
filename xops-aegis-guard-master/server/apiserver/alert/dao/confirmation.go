package dao

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"xops-aegis-guard/pkg/db"
	"xops-aegis-guard/server/apiserver/alert/model"
	alerttools "xops-aegis-guard/server/apiserver/alert/tools"
	jsonutil "xops-aegis-guard/util/json"
)

// MigrateConfirmations is restart-safe: never inherit aggregate metadata onto
// occurrences collected after this aggregate's one-time migration.
func MigrateConfirmations() error {
	var ids []uint
	if err := db.GetOrm().Model(&model.SecurityAlert{}).Where("confirmation_migrated = ?", false).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := db.GetOrm().Transaction(func(tx *gorm.DB) error {
			var row model.SecurityAlert
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
				return err
			}
			return migrateConfirmation(tx, &row)
		}); err != nil {
			return err
		}
	}
	return nil
}

func migrateConfirmation(tx *gorm.DB, alert *model.SecurityAlert) error {
	if alert.ConfirmationMigrated {
		return nil
	}
	var rows []model.AlertOccurrence
	if err := tx.Where("alert_id = ?", alert.ID).Find(&rows).Error; err != nil {
		return err
	}
	var represented int64
	inheritAggregate := alert.Status == "confirmed"
	for i := range rows {
		represented += rows[i].RepresentedCount
		// Existing detail confirmations are authoritative. A legacy aggregate's
		// last confirmation must not fill still-pending details in a partial group.
		if rows[i].ConfirmedAt != nil || rows[i].ConfirmBatchID != "" {
			inheritAggregate = false
		}
	}
	// Earlier versions retained only a count and the latest snapshot. Preserve
	// that count explicitly rather than inventing missing occurrence timestamps.
	if missing := alert.OccurrenceCount - represented; missing > 0 || len(rows) == 0 {
		if missing < 1 {
			missing = 1
		}
		at := alert.LastSeenAt
		if alert.LastProviderTime != nil {
			at = *alert.LastProviderTime
		}
		if at.IsZero() {
			at = time.Now()
		}
		key := sha256.Sum256([]byte(fmt.Sprintf("confirmation-snapshot:%d", alert.ID)))
		eventKey := key[:]
		if len(rows) == 0 {
			// Seed the collector's current event key so the first refresh of
			// an old snapshot cannot reopen an already confirmed alert.
			payload := map[string]any{}
			_ = jsonutil.Unmarshal([]byte(alert.LastPayload), &payload)
			eventKey = alerttools.EventKey(alert.AlertType, alert.Fingerprint, alert.ProviderAlertID, alert.LastProviderTime, payload)
		}
		snapshot := model.AlertOccurrence{AlertID: alert.ID, EventKey: eventKey, OccurredAt: at.Truncate(time.Second), Snapshot: true, RepresentedCount: missing}
		if err := tx.Create(&snapshot).Error; err != nil {
			return err
		}
		rows = append(rows, snapshot)
	}
	for i := range rows {
		row := &rows[i]
		if row.ConfirmedAt == nil && inheritAggregate {
			row.ConfirmedByID = alert.ConfirmedByID
			row.ConfirmedBy = alert.ConfirmedBy
			row.ConfirmedAt = alert.ConfirmedAt
			row.ConfirmNote = alert.ConfirmNote
			if row.ConfirmedBy == "" {
				row.ConfirmedBy = "历史确认"
			}
			if row.ConfirmedAt == nil {
				row.ConfirmedAt = alert.UpdatedAt
				if row.ConfirmedAt == nil {
					row.ConfirmedAt = alert.CreatedAt
				}
				if row.ConfirmedAt == nil {
					at := time.Now().Truncate(time.Second)
					row.ConfirmedAt = &at
				}
			}
		}
		if row.ConfirmedAt == nil || row.ConfirmBatchID != "" {
			continue
		}
		// Exact tuple grouping: username, timestamp to seconds, and note. IDs are
		// intentionally excluded, as required by the historical compatibility rule.
		tuple := fmt.Sprintf("%d:%q:%q:%q", alert.ID, row.ConfirmedBy, row.ConfirmedAt.Truncate(time.Second).Format(time.RFC3339), row.ConfirmNote)
		key := sha256.Sum256([]byte(tuple))
		row.ConfirmBatchID = hex.EncodeToString(key[:])
		if err := tx.Model(row).Updates(map[string]any{"confirm_batch_id": row.ConfirmBatchID, "confirmed_by_id": row.ConfirmedByID, "confirmed_by": row.ConfirmedBy, "confirmed_at": row.ConfirmedAt, "confirm_note": row.ConfirmNote}).Error; err != nil {
			return err
		}
	}
	if err := tx.Model(alert).UpdateColumn("confirmation_migrated", true).Error; err != nil {
		return err
	}
	alert.ConfirmationMigrated = true
	return refreshConfirmation(tx, alert.ID)
}

func refreshConfirmation(tx *gorm.DB, id uint) error {
	var summary struct {
		Total     int64
		Confirmed int64
		Pending   int64
		Batches   int64
	}
	if err := tx.Model(&model.AlertOccurrence{}).Where("alert_id = ?", id).Select(`COALESCE(SUM(represented_count),0) AS total,
 COALESCE(SUM(CASE WHEN confirmed_at IS NOT NULL THEN represented_count ELSE 0 END),0) AS confirmed,
 COALESCE(SUM(CASE WHEN confirmed_at IS NULL THEN represented_count ELSE 0 END),0) AS pending,
 COUNT(DISTINCT NULLIF(confirm_batch_id,'')) AS batches`).Scan(&summary).Error; err != nil {
		return err
	}
	updates := map[string]any{"confirmation_count": summary.Batches, "confirmed_count": summary.Confirmed, "pending_count": summary.Pending, "occurrence_count": summary.Total}
	if summary.Pending == 0 && summary.Total > 0 {
		updates["status"] = "confirmed"
	} else {
		updates["status"] = gorm.Expr("CASE WHEN status = 'confirmed' THEN 'firing' ELSE status END")
	}
	if summary.Confirmed > 0 {
		var latest model.AlertOccurrence
		if err := tx.Where("alert_id = ? AND confirmed_at IS NOT NULL", id).Order("confirmed_at DESC, id DESC").First(&latest).Error; err != nil {
			return err
		}
		updates["confirmed_by_id"] = latest.ConfirmedByID
		updates["confirmed_by"] = latest.ConfirmedBy
		updates["confirmed_at"] = latest.ConfirmedAt
		updates["confirm_note"] = latest.ConfirmNote
	}
	return tx.Model(&model.SecurityAlert{}).Where("id = ?", id).Updates(updates).Error
}

// ConfirmOccurrences locks aggregates in stable order, just like collection.
// Only pending details receive this submission's batch; a no-op changes nothing.
func ConfirmOccurrences(ids []uint, occurrenceID uint, note string, byID int, by string, at time.Time) ([]uint, error) {
	unique := map[uint]bool{}
	ordered := make([]uint, 0, len(ids))
	for _, id := range ids {
		if !unique[id] {
			unique[id] = true
			ordered = append(ordered, id)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	var changed []uint
	err := db.GetOrm().Transaction(func(tx *gorm.DB) error {
		alerts := make([]model.SecurityAlert, 0, len(ordered))
		for _, id := range ordered {
			var alert model.SecurityAlert
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&alert, id).Error; err != nil {
				return err
			}
			if err := migrateConfirmation(tx, &alert); err != nil {
				return err
			}
			alerts = append(alerts, alert)
		}
		batch := ""
		for _, alert := range alerts {
			query := tx.Model(&model.AlertOccurrence{}).Where("alert_id = ? AND confirmed_at IS NULL", alert.ID)
			if occurrenceID != 0 {
				var row model.AlertOccurrence
				if err := tx.Where("id = ? AND alert_id = ?", occurrenceID, alert.ID).First(&row).Error; err != nil {
					return err
				}
				query = query.Where("id = ?", occurrenceID)
			}
			var count int64
			if err := query.Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				continue
			}
			if batch == "" {
				var bytes [32]byte
				if _, err := rand.Read(bytes[:]); err != nil {
					return err
				}
				batch = hex.EncodeToString(bytes[:])
			}
			if err := query.Updates(map[string]any{"confirm_batch_id": batch, "confirmed_by_id": byID, "confirmed_by": by, "confirmed_at": at.Truncate(time.Second), "confirm_note": note}).Error; err != nil {
				return err
			}
			if err := refreshConfirmation(tx, alert.ID); err != nil {
				return err
			}
			// If two submissions land in the same second, the actual last
			// submission still supplies the aggregate's latest actor and note.
			if err := tx.Model(&model.SecurityAlert{}).Where("id = ? AND (confirmed_at IS NULL OR confirmed_at <= ?)", alert.ID, at.Truncate(time.Second)).Updates(map[string]any{
				"confirmed_by_id": byID, "confirmed_by": by, "confirmed_at": at.Truncate(time.Second), "confirm_note": note,
			}).Error; err != nil {
				return err
			}
			changed = append(changed, alert.ID)
		}
		return nil
	})
	return changed, err
}
