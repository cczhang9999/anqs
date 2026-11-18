package dao

import (
	"xops-aegis-guard/pkg/db"
	"xops-aegis-guard/server/apiserver/device/model"
)

// MigrateDeviceTable removes obsolete device snapshot fields. Any data needed
// by device matching or presentation is stored in a dedicated typed column.
func MigrateDeviceTable() error {
	migrator := db.GetOrm().Migrator()
	table := &model.Device{}
	for _, column := range []string{"region_id", "region_name", "public_ip", "status", "payload_json", "hostname", "last_seen_at", "last_sync_at"} {
		if migrator.HasColumn(table, column) {
			if err := migrator.DropColumn(table, column); err != nil {
				return err
			}
		}
	}
	return nil
}
