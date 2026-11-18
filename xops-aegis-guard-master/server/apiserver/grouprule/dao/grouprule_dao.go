package dao

import (
	"errors"
	"strings"

	"xops-aegis-guard/pkg/db"
	"xops-aegis-guard/server/apiserver/grouprule/model"

	"gorm.io/gorm"
)

func IsNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

// List 按筛选条件分页读取规则，执行顺序为全局升序。
func List(keyword string, departmentID *uint, limit, offset int) ([]model.GroupRule, int64, error) {
	query := db.GetOrm().Model(&model.GroupRule{})
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if departmentID != nil {
		query = query.Where("department_id = ?", *departmentID)
	}
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
	var rows []model.GroupRule
	err := query.Order("sort_order ASC, id ASC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

// All 返回全部规则（含停用），用于顺序统计和展示。
func All() ([]model.GroupRule, error) {
	var rows []model.GroupRule
	err := db.GetOrm().Order("sort_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

// ListEnabled 返回参与匹配的规则，按执行顺序升序。
func ListEnabled() ([]model.GroupRule, error) {
	var rows []model.GroupRule
	err := db.GetOrm().Where("enabled = ?", true).Order("sort_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

func Get(id uint) (*model.GroupRule, error) {
	var row model.GroupRule
	err := db.GetOrm().First(&row, id).Error
	return &row, err
}

func Save(row *model.GroupRule) error { return db.GetOrm().Save(row).Error }

// UpdateFields 只更新允许编辑的字段，避免覆盖顺序和创建时间。
func UpdateFields(id uint, fields map[string]interface{}) error {
	return db.GetOrm().Model(&model.GroupRule{}).Where("id = ?", id).Updates(fields).Error
}

func SetEnabled(id uint, enabled bool) error {
	return UpdateFields(id, map[string]interface{}{"enabled": enabled})
}

// Delete 软删除规则，并把剩余规则的顺序号重排为 0..n-1。
func Delete(id uint) error {
	return db.GetOrm().Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.GroupRule{}, id).Error; err != nil {
			return err
		}
		var rows []model.GroupRule
		if err := tx.Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			if rows[i].SortOrder == i {
				continue
			}
			if err := tx.Model(&model.GroupRule{}).Where("id = ?", rows[i].ID).Update("sort_order", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// MaxSortOrder 返回当前最大顺序号，新规则追加到末尾。
func MaxSortOrder() (int, error) {
	var row struct {
		MaxSort *int
	}
	err := db.GetOrm().Model(&model.GroupRule{}).Select("MAX(sort_order) AS max_sort").Scan(&row).Error
	if err != nil {
		return 0, err
	}
	if row.MaxSort == nil {
		return -1, nil
	}
	return *row.MaxSort, nil
}

// Reorder 把规则上移、下移或置顶，并在同一事务里把顺序号重排为 0..n-1。
func Reorder(id uint, action string) error {
	return db.GetOrm().Transaction(func(tx *gorm.DB) error {
		var rows []model.GroupRule
		if err := tx.Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
			return err
		}
		index := -1
		for i := range rows {
			if rows[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return gorm.ErrRecordNotFound
		}
		switch action {
		case "top":
			if index == 0 {
				return nil
			}
			row := rows[index]
			rows = append(rows[:index], rows[index+1:]...)
			rows = append([]model.GroupRule{row}, rows...)
		case "up":
			if index == 0 {
				return nil
			}
			rows[index-1], rows[index] = rows[index], rows[index-1]
		case "down":
			if index == len(rows)-1 {
				return nil
			}
			rows[index+1], rows[index] = rows[index], rows[index+1]
		default:
			return errors.New("unsupported reorder action")
		}
		for i := range rows {
			if rows[i].SortOrder == i {
				continue
			}
			if err := tx.Model(&model.GroupRule{}).Where("id = ?", rows[i].ID).Update("sort_order", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// HostCounts 统计每条规则当前归属的主机数。
func HostCounts(ruleIDs []uint) (map[uint]int64, error) {
	counts := make(map[uint]int64, len(ruleIDs))
	if len(ruleIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		AssignRuleID uint
		Total        int64
	}
	err := db.GetOrm().Table("devices").
		Select("assign_rule_id, COUNT(*) AS total").
		Where("assign_rule_id IN ? AND deleted_at IS NULL", ruleIDs).
		Group("assign_rule_id").Scan(&rows).Error
	if err != nil {
		return counts, err
	}
	for _, row := range rows {
		counts[row.AssignRuleID] = row.Total
	}
	return counts, nil
}

// Stats 返回启用规则数和规则总数。
func Stats() (enabled, total int64, err error) {
	base := db.GetOrm().Session(&gorm.Session{NewDB: true}).Model(&model.GroupRule{})
	if err = base.Count(&total).Error; err != nil {
		return
	}
	if err = db.GetOrm().Session(&gorm.Session{NewDB: true}).Model(&model.GroupRule{}).Where("enabled = ?", true).Count(&enabled).Error; err != nil {
		return
	}
	return
}

// ExistingIDs 返回仍然存在的规则 ID 集合，用于把“规则已删除”的来源标记出来。
func ExistingIDs() (map[uint]struct{}, error) {
	ids := make([]uint, 0)
	if err := db.GetOrm().Model(&model.GroupRule{}).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	set := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

// RuleNames 返回规则 ID → 名称，列表筛选和展示兜底使用。
func RuleNames() (map[uint]string, error) {
	type row struct {
		ID   uint
		Name string
	}
	var rows []row
	if err := db.GetOrm().Model(&model.GroupRule{}).Select("id, name").Scan(&rows).Error; err != nil {
		return nil, err
	}
	names := make(map[uint]string, len(rows))
	for _, item := range rows {
		names[item.ID] = item.Name
	}
	return names, nil
}

// CountByDepartment 统计目标部门被多少条规则引用。
func CountByDepartment(departmentID uint) (int64, error) {
	var count int64
	err := db.GetOrm().Model(&model.GroupRule{}).Where("department_id = ?", departmentID).Count(&count).Error
	return count, err
}

// NameExists 检查同名规则是否已存在（排除自身），避免列表出现两行同名规则。
func NameExists(name string, excludeID uint) (bool, error) {
	query := db.GetOrm().Model(&model.GroupRule{}).Where("name = ?", strings.TrimSpace(name))
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
