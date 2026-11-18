package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"xops-aegis-guard/logger"
	"xops-aegis-guard/pkg/mistake"
	basemodel "xops-aegis-guard/server/apiserver/base/model"
	departmentdao "xops-aegis-guard/server/apiserver/department/dao"
	devicedao "xops-aegis-guard/server/apiserver/device/dao"
	devicemodel "xops-aegis-guard/server/apiserver/device/model"
	"xops-aegis-guard/server/apiserver/grouprule/dao"
	rulemodel "xops-aegis-guard/server/apiserver/grouprule/model"
	"xops-aegis-guard/server/apiserver/grouprule/tools"
	"xops-aegis-guard/server/apiserver/grouprule/val"
	jsonutil "xops-aegis-guard/util/json"
	"xops-aegis-guard/util/pager"

	"gorm.io/gorm"
)

func IsNotFound(err error) bool { return dao.IsNotFound(err) }

// List 分页返回规则，并附带目标部门名称和当前归属主机数。
func List(req *val.ListRuleReq) ([]val.RuleResp, int64, int, int, error) {
	limit, offset := pager.Parse(strconv.Itoa(req.Limit), strconv.Itoa(req.Offset))
	rows, total, err := dao.List(req.Keyword, req.DepartmentID, limit, offset)
	if err != nil {
		return nil, total, limit, offset, err
	}
	items, err := toRuleResps(rows)
	return items, total, limit, offset, err
}

func Get(req *val.GetRuleReq) (*val.RuleResp, error) {
	row, err := dao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	items, err := toRuleResps([]rulemodel.GroupRule{*row})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// Create 新增规则，排序追加到列表末尾。
func Create(req *val.SaveRuleReq) (*val.RuleResp, error) {
	if _, err := departmentdao.Get(req.DepartmentID); err != nil {
		return nil, mistake.NewReqErr("目标部门不存在")
	}
	conditions := req.NormalizedConditions()
	if _, err := tools.CompileConditions(conditions); err != nil {
		return nil, mistake.NewReqErr(err.Error())
	}
	encoded, err := jsonutil.Marshal(conditions)
	if err != nil {
		return nil, err
	}
	maxSort, err := dao.MaxSortOrder()
	if err != nil {
		return nil, err
	}
	row := &rulemodel.GroupRule{
		Name:         strings.TrimSpace(req.Name),
		DepartmentID: req.DepartmentID,
		Enabled:      enabledOrDefault(req.Enabled, true),
		SortOrder:    maxSort + 1,
		Conditions:   basemodel.JSON(encoded),
	}
	if err := dao.Save(row); err != nil {
		return nil, err
	}
	resp, err := toRuleResps([]rulemodel.GroupRule{*row})
	if err != nil {
		return nil, err
	}
	return &resp[0], nil
}

// Update 编辑规则内容，不改变执行顺序。
func Update(req *val.SaveRuleReq) (*val.RuleResp, error) {
	if req.ID == 0 {
		return nil, mistake.NewReqErr("invalid id")
	}
	if _, err := dao.Get(req.ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, mistake.NewReqErr("规则不存在")
		}
		return nil, err
	}
	if _, err := departmentdao.Get(req.DepartmentID); err != nil {
		return nil, mistake.NewReqErr("目标部门不存在")
	}
	conditions := req.NormalizedConditions()
	if _, err := tools.CompileConditions(conditions); err != nil {
		return nil, mistake.NewReqErr(err.Error())
	}
	encoded, err := jsonutil.Marshal(conditions)
	if err != nil {
		return nil, err
	}
	fields := map[string]interface{}{
		"name":          strings.TrimSpace(req.Name),
		"department_id": req.DepartmentID,
		"conditions":    basemodel.JSON(encoded),
	}
	if req.Enabled != nil {
		fields["enabled"] = *req.Enabled
	}
	if err := dao.UpdateFields(req.ID, fields); err != nil {
		return nil, err
	}
	row, err := dao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	resp, err := toRuleResps([]rulemodel.GroupRule{*row})
	if err != nil {
		return nil, err
	}
	return &resp[0], nil
}

// SetEnabled 切换启用状态；停用后该规则不再参与匹配。
func SetEnabled(req *val.ToggleRuleReq) error {
	if req.Enabled == nil {
		return mistake.NewReqErr("缺少启用状态")
	}
	if _, err := dao.Get(req.ID); err != nil {
		return mistake.NewReqErr("规则不存在")
	}
	return dao.SetEnabled(req.ID, *req.Enabled)
}

// Reorder 置顶 / 上移 / 下移。
func Reorder(req *val.ReorderRuleReq) error {
	if err := dao.Reorder(req.ID, req.Action); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mistake.NewReqErr("规则不存在")
		}
		return err
	}
	return nil
}

// Delete 删除规则。已经按该规则归属的主机保持现有部门不变。
func Delete(req *val.DeleteRuleReq) (*val.DeleteResp, error) {
	if _, err := dao.Get(req.ID); err != nil {
		return nil, mistake.NewReqErr("规则不存在")
	}
	counts, err := dao.HostCounts([]uint{req.ID})
	if err != nil {
		return nil, err
	}
	if err := dao.Delete(req.ID); err != nil {
		return nil, err
	}
	return &val.DeleteResp{Affected: counts[req.ID]}, nil
}

// Stats 返回规则页统计卡数据。
func Stats(req *val.RuleStatsReq) (*val.RuleStatsResp, error) {
	enabled, total, err := dao.Stats()
	if err != nil {
		return nil, err
	}
	ruleHosts, manualHosts, unassignedHosts, err := devicedao.SourceStats()
	if err != nil {
		return nil, err
	}
	return &val.RuleStatsResp{
		EnabledRules:    enabled,
		TotalRules:      total,
		RuleHosts:       ruleHosts,
		ManualHosts:     manualHosts,
		UnassignedHosts: unassignedHosts,
	}, nil
}

// Preview 试运行：计算待清理的失效归属及重新匹配结果，不写库。
func Preview(req *val.PreviewRuleReq) (*val.PreviewResp, error) {
	limit, offset := pager.Parse(strconv.Itoa(req.Limit), strconv.Itoa(req.Offset))
	totalUnassigned, plan, stale, err := buildPlan()
	if err != nil {
		return nil, err
	}
	names, err := departmentNames()
	if err != nil {
		return nil, err
	}
	matched := int64(len(plan))
	items := make([]val.PreviewItem, 0, limit)
	for index := offset; index < len(plan) && len(items) < limit; index++ {
		item := plan[index]
		items = append(items, val.PreviewItem{
			DeviceID:            item.device.ID,
			InstanceName:        item.device.InstanceName,
			PrivateIP:           item.device.PrivateIP,
			CurrentDepartmentID: item.device.DepartmentID,
			DepartmentID:        item.rule.DepartmentID,
			DepartmentName:      names[item.rule.DepartmentID],
			RuleID:              item.rule.ID,
			RuleName:            item.rule.Name,
		})
	}
	return &val.PreviewResp{
		TotalUnassigned: totalUnassigned,
		MatchedTotal:    matched,
		ClearedTotal:    int64(len(stale)),
		Items:           items,
		Limit:           limit,
		Offset:          offset,
	}, nil
}

// Execute 立即执行：清理失效归属，再写入命中规则的主机归属。
func Execute(req *val.ExecuteRuleReq) (*val.ExecuteResp, error) {
	assigned, cleared, err := executePlan()
	if err != nil {
		return nil, err
	}
	return &val.ExecuteResp{Assigned: assigned, Cleared: cleared}, nil
}

func executePlan() (int64, int64, error) {
	total, plan, stale, err := buildPlan()
	if err != nil {
		return 0, 0, err
	}
	assignments := make([]devicedao.RuleAssignment, 0, len(plan))
	for _, item := range plan {
		assignments = append(assignments, devicedao.RuleAssignment{
			DeviceID:     item.device.ID,
			InstanceName: item.device.InstanceName,
			PrivateIP:    item.device.PrivateIP,
			DepartmentID: item.rule.DepartmentID,
			RuleID:       item.rule.ID,
			RuleName:     item.rule.Name,
		})
	}
	assigned, cleared, err := devicedao.ApplyRuleAssignments(assignments, stale, time.Now())
	if err != nil {
		return 0, 0, err
	}
	logger.Infof("group rules executed: unassigned=%d matched=%d assigned=%d cleared=%d", total, len(plan), assigned, cleared)
	return assigned, cleared, nil
}

type compiledRule struct {
	rule       rulemodel.GroupRule
	conditions []tools.CompiledCondition
}

type plannedAssignment struct {
	device devicemodel.Device
	rule   rulemodel.GroupRule
}

// buildPlan 复核已有自动归属，再计算未分配主机命中的第一条启用规则，只读不写。
func buildPlan() (int64, []plannedAssignment, []devicemodel.Device, error) {
	rules, err := compileRules()
	if err != nil {
		return 0, nil, nil, err
	}
	devices, err := devicedao.ListGroupingCandidates()
	if err != nil {
		return 0, nil, nil, err
	}
	total, plan, stale := planDevices(rules, devices)
	return total, plan, stale, nil
}

func planDevices(rules []compiledRule, devices []devicemodel.Device) (int64, []plannedAssignment, []devicemodel.Device) {
	byID := make(map[uint]compiledRule, len(rules))
	for _, rule := range rules {
		byID[rule.rule.ID] = rule
	}
	var totalUnassigned int64
	plan := make([]plannedAssignment, 0, len(devices))
	stale := make([]devicemodel.Device, 0)
	for _, device := range devices {
		hostname := strings.TrimSpace(device.InstanceName)
		if device.DepartmentID != 0 {
			switch device.AssignSource {
			case "":
				// 有部门却没有来源不是有效归属，重新交给规则判断。
			case devicemodel.AssignSourceRule:
				previous, exists := byID[device.AssignRuleID]
				// 删除或无法解析的规则保留历史归属；停用只阻止新增匹配。
				if !exists || (previous.rule.DepartmentID == device.DepartmentID && tools.MatchDevice(previous.conditions, hostname, device.PrivateIP)) {
					continue
				}
			default:
				continue // 手动分配不参与自动校正。
			}
			stale = append(stale, device)
		}
		totalUnassigned++
		for _, rule := range rules {
			if rule.rule.Enabled && tools.MatchDevice(rule.conditions, hostname, device.PrivateIP) {
				plan = append(plan, plannedAssignment{device: device, rule: rule.rule})
				break
			}
		}
	}
	return totalUnassigned, plan, stale
}

// compileRules 按顺序预编译全部规则，包含用于复核历史归属的停用规则。
func compileRules() ([]compiledRule, error) {
	rows, err := dao.All()
	if err != nil {
		return nil, err
	}
	result := make([]compiledRule, 0, len(rows))
	for _, row := range rows {
		conditions, err := decodeConditions(row)
		if err != nil {
			logger.Warnf("skip invalid group rule: id=%d name=%s error=%v", row.ID, row.Name, err)
			continue
		}
		compiled, err := tools.CompileConditions(conditions)
		if err != nil {
			logger.Warnf("skip invalid group rule: id=%d name=%s error=%v", row.ID, row.Name, err)
			continue
		}
		result = append(result, compiledRule{rule: row, conditions: compiled})
	}
	return result, nil
}

func toRuleResps(rows []rulemodel.GroupRule) ([]val.RuleResp, error) {
	names, err := departmentNames()
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	counts, err := dao.HostCounts(ids)
	if err != nil {
		return nil, err
	}
	items := make([]val.RuleResp, 0, len(rows))
	for _, row := range rows {
		conditions, err := decodeConditions(row)
		if err != nil {
			return nil, fmt.Errorf("解析规则 %s 的匹配条件失败: %w", row.Name, err)
		}
		items = append(items, val.RuleResp{
			ID:             row.ID,
			Name:           row.Name,
			DepartmentID:   row.DepartmentID,
			DepartmentName: names[row.DepartmentID],
			Enabled:        row.Enabled,
			SortOrder:      row.SortOrder,
			Conditions:     conditions,
			HostCount:      counts[row.ID],
		})
	}
	return items, nil
}

func decodeConditions(row rulemodel.GroupRule) ([]rulemodel.Condition, error) {
	if len(row.Conditions) == 0 {
		return []rulemodel.Condition{}, nil
	}
	var conditions []rulemodel.Condition
	if err := jsonutil.Unmarshal([]byte(row.Conditions), &conditions); err != nil {
		return nil, err
	}
	return conditions, nil
}

func departmentNames() (map[uint]string, error) {
	rows, err := departmentdao.List()
	if err != nil {
		return nil, err
	}
	names := make(map[uint]string, len(rows))
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}

func enabledOrDefault(enabled *bool, fallback bool) bool {
	if enabled == nil {
		return fallback
	}
	return *enabled
}
