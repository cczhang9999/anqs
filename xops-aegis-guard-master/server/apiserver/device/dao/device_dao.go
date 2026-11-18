package dao

import (
	"errors"
	"strings"
	"time"

	"xops-aegis-guard/pkg/db"
	"xops-aegis-guard/server/apiserver/device/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func IsNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

func Upsert(device *model.Device) error {
	var existing model.Device
	orm := db.GetOrm()
	err := orm.Where("provider = ? AND account_type = ? AND aliyun_uuid = ?", device.Provider, device.AccountType, device.AliyunUUID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return orm.Create(device).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"instance_id": device.InstanceID,
		"private_ip":  device.PrivateIP,
		"os_name":     device.OSName,
	}
	// Security Center can intermittently omit InstanceName from an asset
	// snapshot. Do not erase a name already learned from an earlier sync.
	if strings.TrimSpace(device.InstanceName) != "" {
		updates["instance_name"] = device.InstanceName
	}
	return orm.Model(&existing).Updates(updates).Error
}

func List(limit, offset int, keyword, ip string, departmentID *uint, source string) ([]model.Device, int64, error) {
	query := db.GetOrm().Model(&model.Device{})
	if keyword != "" {
		like := "%" + keyword + "%"
		// The UI's keyword field is for host / instance names. Keep IPs in the
		// dedicated IP filter and avoid matching internal IDs or cloud UUIDs,
		// which can otherwise return hosts whose displayed name is unrelated.
		query = query.Where("instance_name LIKE ?", like)
	}
	if ip != "" {
		like := "%" + ip + "%"
		query = query.Where("private_ip LIKE ?", like)
	}
	if departmentID != nil {
		query = query.Where("department_id = ?", *departmentID)
	}
	switch source {
	case "unassigned":
		query = query.Where("department_id = ?", 0)
	case model.AssignSourceRule, model.AssignSourceManual:
		query = query.Where("department_id > ? AND assign_source = ?", 0, source)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []model.Device
	err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func Stats() (total, assigned, unassigned int64, err error) {
	base := db.GetOrm().Session(&gorm.Session{NewDB: true}).Model(&model.Device{})
	if err = base.Count(&total).Error; err != nil {
		return
	}
	if err = db.GetOrm().Session(&gorm.Session{NewDB: true}).Model(&model.Device{}).Where("department_id > 0").Count(&assigned).Error; err != nil {
		return
	}
	if err = db.GetOrm().Session(&gorm.Session{NewDB: true}).Model(&model.Device{}).Where("department_id = 0").Count(&unassigned).Error; err != nil {
		return
	}
	return
}

func All() ([]model.Device, error) {
	var rows []model.Device
	err := db.GetOrm().Order("id DESC").Find(&rows).Error
	return rows, err
}

func Simple() ([]model.DeviceSimple, error) {
	var rows []model.DeviceSimple
	err := db.GetOrm().Model(&model.Device{}).
		Select("id, instance_name AS name, private_ip, department_id").
		Order("id DESC").Find(&rows).Error
	return rows, err
}

// ListAlertScopeIDs 返回满足告警筛选和可见范围的设备 ID。告警模块使用
// 该结果限制 security_alerts，避免在告警列表 SQL 中关联 devices 表。
func ListAlertScopeIDs(keyword, ip string, departmentID *uint, deviceID uint, allowedDepartmentIDs []uint) ([]uint, error) {
	query := db.GetOrm().Model(&model.Device{})
	if len(allowedDepartmentIDs) == 0 {
		query = query.Where("department_id = 0")
	} else {
		query = query.Where("department_id = 0 OR department_id IN ?", allowedDepartmentIDs)
	}
	if departmentID != nil {
		query = query.Where("department_id = ?", *departmentID)
	}
	if deviceID != 0 {
		query = query.Where("id = ?", deviceID)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("instance_name LIKE ? OR instance_id LIKE ? OR private_ip LIKE ?", like, like, like)
	}
	if ip != "" {
		query = query.Where("private_ip LIKE ?", "%"+ip+"%")
	}
	ids := make([]uint, 0)
	err := query.Order("id DESC").Pluck("id", &ids).Error
	return ids, err
}

// ListByIDs 批量读取当前告警页需要展示的设备资料。
func ListByIDs(ids []uint) ([]model.Device, error) {
	if len(ids) == 0 {
		return []model.Device{}, nil
	}
	var rows []model.Device
	err := db.GetOrm().Where("id IN ?", ids).Find(&rows).Error
	return rows, err
}

func Get(id uint) (*model.Device, error) {
	var row model.Device
	err := db.GetOrm().First(&row, id).Error
	return &row, err
}

func Save(row *model.Device) error { return db.GetOrm().Save(row).Error }

func Delete(id uint) error { return db.GetOrm().Delete(&model.Device{}, id).Error }

func Find(accountType, uuid, instanceID, name, privateIP string) (*model.Device, error) {
	var row model.Device
	conditions := make([]string, 0, 5)
	args := []any{"aliyun", accountType}
	for _, item := range []struct{ field, value string }{{"aliyun_uuid", uuid}, {"instance_id", instanceID}, {"instance_name", name}, {"private_ip", privateIP}} {
		if item.value == "" {
			continue
		}
		conditions = append(conditions, item.field+" = ?")
		args = append(args, item.value)
	}
	if len(conditions) > 0 {
		// Find does not emit GORM's noisy record-not-found error log for normal
		// best-effort association misses.
		query := db.GetOrm().Where("provider = ? AND account_type = ? AND ("+strings.Join(conditions, " OR ")+")", args...)
		if err := query.Limit(1).Find(&row).Error; err == nil && row.ID != 0 {
			return &row, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

// SourceStats 按归属来源统计主机数，供分组规则页的统计卡使用。
func SourceStats() (rule, manual, unassigned int64, err error) {
	if err = db.GetOrm().Session(&gorm.Session{NewDB: true}).Model(&model.Device{}).
		Where("department_id > 0 AND assign_source = ?", model.AssignSourceRule).Count(&rule).Error; err != nil {
		return
	}
	if err = db.GetOrm().Session(&gorm.Session{NewDB: true}).Model(&model.Device{}).
		Where("department_id > 0 AND assign_source = ?", model.AssignSourceManual).Count(&manual).Error; err != nil {
		return
	}
	if err = db.GetOrm().Session(&gorm.Session{NewDB: true}).Model(&model.Device{}).
		Where("department_id = ?", 0).Count(&unassigned).Error; err != nil {
		return
	}
	return
}

// ListGroupingCandidates 同时读取未分配主机及需要复核的自动/无来源归属。
func ListGroupingCandidates() ([]model.Device, error) {
	var rows []model.Device
	err := db.GetOrm().Where("department_id = ? OR assign_source IN ?", 0, []string{model.AssignSourceRule, ""}).
		Order("id ASC").Find(&rows).Error
	return rows, err
}

// RuleAssignment 描述一次规则归属落库。
type RuleAssignment struct {
	DeviceID     uint
	InstanceName string
	PrivateIP    string
	DepartmentID uint
	RuleID       uint
	RuleName     string
}

// ApplyRuleAssignments 在同一事务中清理失效归属，再批量写入匹配结果。
// 清理时核对快照，分配时锁定并复核名称和 IP，避免覆盖并发人工分配或设备变化。
func ApplyRuleAssignments(assignments []RuleAssignment, stale []model.Device, assignedAt time.Time) (int64, int64, error) {
	if len(assignments) == 0 && len(stale) == 0 {
		return 0, 0, nil
	}
	grouped := make(map[uint][]RuleAssignment)
	for _, item := range assignments {
		key := item.RuleID
		grouped[key] = append(grouped[key], item)
	}
	var updated, cleared int64
	err := db.GetOrm().Transaction(func(tx *gorm.DB) error {
		for _, device := range stale {
			result := tx.Model(&model.Device{}).Where(map[string]interface{}{
				"id": device.ID, "department_id": device.DepartmentID,
				"assign_source": device.AssignSource, "assign_rule_id": device.AssignRuleID,
				"updated_at": device.UpdatedAt,
			}).Where("BINARY instance_name = ? AND BINARY private_ip = ?", device.InstanceName, device.PrivateIP).Updates(assignFields(0, ""))
			if result.Error != nil {
				return result.Error
			}
			cleared += result.RowsAffected
		}
		for _, items := range grouped {
			ids := make([]uint, 0, len(items))
			snapshots := make(map[uint]RuleAssignment, len(items))
			for _, item := range items {
				ids = append(ids, item.DeviceID)
				snapshots[item.DeviceID] = item
			}
			first := items[0]
			for start := 0; start < len(ids); start += 500 {
				end := start + 500
				if end > len(ids) {
					end = len(ids)
				}
				var current []model.Device
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Select("id, instance_name, private_ip").Where("id IN ? AND department_id = ?", ids[start:end], 0).
					Find(&current).Error; err != nil {
					return err
				}
				validIDs := make([]uint, 0, len(current))
				for _, device := range current {
					snapshot := snapshots[device.ID]
					if device.InstanceName == snapshot.InstanceName && device.PrivateIP == snapshot.PrivateIP {
						validIDs = append(validIDs, device.ID)
					}
				}
				if len(validIDs) == 0 {
					continue
				}
				result := tx.Model(&model.Device{}).
					Where("id IN ? AND department_id = ?", validIDs, 0).
					Updates(map[string]interface{}{
						"department_id":    first.DepartmentID,
						"assign_source":    model.AssignSourceRule,
						"assign_rule_id":   first.RuleID,
						"assign_rule_name": first.RuleName,
						"assign_operator":  "",
						"assigned_at":      assignedAt,
					})
				if result.Error != nil {
					return result.Error
				}
				updated += result.RowsAffected
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return updated, cleared, nil
}

// AssignDepartment 人工分配主机到部门；departmentID 为 0 表示交还给规则。
func AssignDepartment(id, departmentID uint, operator string) error {
	return db.GetOrm().Model(&model.Device{}).Where("id = ?", id).Updates(assignFields(departmentID, operator)).Error
}

// BatchAssignDepartment 批量人工分配主机到部门。
func BatchAssignDepartment(ids []uint, departmentID uint, operator string) error {
	return db.GetOrm().Model(&model.Device{}).Where("id IN ?", ids).Updates(assignFields(departmentID, operator)).Error
}

// assignFields 生成归属字段：部门为空时清空来源，交给下一次规则执行。
func assignFields(departmentID uint, operator string) map[string]interface{} {
	if departmentID == 0 {
		return map[string]interface{}{
			"department_id":    uint(0),
			"assign_source":    "",
			"assign_rule_id":   uint(0),
			"assign_rule_name": "",
			"assign_operator":  "",
			"assigned_at":      nil,
		}
	}
	return map[string]interface{}{
		"department_id":    departmentID,
		"assign_source":    model.AssignSourceManual,
		"assign_rule_id":   uint(0),
		"assign_rule_name": "",
		"assign_operator":  operator,
		"assigned_at":      time.Now(),
	}
}
