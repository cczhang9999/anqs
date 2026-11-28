package dao

import (
	"errors"
	"strconv"

	"xops-aegis-guard/pkg/db"
	"xops-aegis-guard/server/apiserver/department/model"

	"gorm.io/gorm"
)

func IsNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

func Get(id uint) (*model.Department, error) {
	var row model.Department
	err := db.GetOrm().First(&row, id).Error
	return &row, err
}

func List() ([]model.Department, error) {
	var rows []model.Department
	err := db.GetOrm().Order("id DESC").Find(&rows).Error
	return rows, err
}

// ListByMember 只返回 member_ids 里包含该用户的部门。用户无效时返回空列表，不查全表。
func ListByMember(userID int) ([]model.Department, error) {
	if userID <= 0 {
		return []model.Department{}, nil
	}
	var rows []model.Department
	err := db.GetOrm().
		Where("JSON_CONTAINS(member_ids, ?)", strconv.Itoa(userID)).
		Order("id DESC").
		Find(&rows).Error
	return rows, err
}

func ListPage(limit, offset int) ([]model.Department, int64, error) {
	query := db.GetOrm().Model(&model.Department{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var rows []model.Department
	err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func Save(row *model.Department) error { return db.GetOrm().Save(row).Error }

func Delete(id uint) error {
	return db.GetOrm().Transaction(func(tx *gorm.DB) error {
		// 删除部门时解除设备的部门归属，并把归属来源一起清空，交还给分组规则。
		if err := tx.Table("devices").Where("department_id = ?", id).Updates(map[string]interface{}{
			"department_id":    0,
			"assign_source":    "",
			"assign_rule_id":   0,
			"assign_rule_name": "",
			"assign_operator":  "",
			"assigned_at":      nil,
		}).Error; err != nil {
			return err
		}
		// 旧版本告警表带有部门快照列，当前版本已移除，只有列存在时才同步清理。
		migrator := tx.Migrator()
		alertTable := "security_alerts"
		if migrator.HasTable(alertTable) && migrator.HasColumn(alertTable, "department_id") {
			updates := map[string]interface{}{"department_id": 0}
			if migrator.HasColumn(alertTable, "department_snapshot") {
				updates["department_snapshot"] = ""
			}
			if err := tx.Table(alertTable).Where("department_id = ?", id).Updates(updates).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&model.Department{}, id).Error
	})
}
