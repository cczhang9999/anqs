package val

import (
	"fmt"
	"strings"

	"xops-aegis-guard/pkg/user"
	"xops-aegis-guard/server/apiserver/grouprule/model"
	"xops-aegis-guard/server/apiserver/grouprule/tools"
)

// 规则数量和文本上限，避免一条规则塞入过多条件拖慢每次执行。
const (
	MaxConditions         = 20
	MaxValuesPerCondition = 50
	MaxConditionValueLen  = 512
	MaxRuleNameLen        = 30
)

type SaveRuleReq struct {
	AuthUser     *user.AuthUser    `json:"-"`
	ID           uint              `json:"id"`
	Name         string            `validate:"required,lte=64" comment:"规则名称" json:"name"`
	DepartmentID uint              `validate:"required,min=1" comment:"目标部门" json:"department_id"`
	Enabled      *bool             `json:"enabled"`
	Conditions   []model.Condition `json:"conditions"`
}

type ListRuleReq struct {
	AuthUser     *user.AuthUser `json:"-"`
	Limit        int            `validate:"omitempty,min=0,max=200" comment:"每页数量" form:"limit"`
	Offset       int            `validate:"omitempty,min=0" comment:"偏移量" form:"offset"`
	Keyword      string         `validate:"omitempty,lte=128" comment:"规则名称" form:"keyword"`
	DepartmentID *uint          `validate:"omitempty,min=0" comment:"目标部门" form:"department_id"`
}

type ToggleRuleReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `validate:"required,min=1" json:"id" form:"id" binding:"required" comment:"规则ID"`
	Enabled  *bool          `json:"enabled" form:"enabled" comment:"是否启用"`
}

type ReorderRuleReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `validate:"required,min=1" json:"id" form:"id" binding:"required" comment:"规则ID"`
	Action   string         `validate:"required,oneof=top up down" json:"action" form:"action" binding:"required" comment:"调整动作"`
}

type DeleteRuleReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `validate:"required,min=1" json:"id" form:"id" binding:"required" comment:"规则ID"`
}

type GetRuleReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `json:"-"`
}

type PreviewRuleReq struct {
	AuthUser *user.AuthUser `json:"-"`
	Limit    int            `validate:"omitempty,min=0,max=200" comment:"每页数量" form:"limit"`
	Offset   int            `validate:"omitempty,min=0" comment:"偏移量" form:"offset"`
}

type ExecuteRuleReq struct {
	AuthUser *user.AuthUser `json:"-"`
}

type RuleStatsReq struct {
	AuthUser *user.AuthUser `json:"-"`
}

type RuleResp struct {
	ID             uint              `json:"id"`
	Name           string            `json:"name"`
	DepartmentID   uint              `json:"department_id"`
	DepartmentName string            `json:"department_name"`
	Enabled        bool              `json:"enabled"`
	SortOrder      int               `json:"sort_order"`
	Conditions     []model.Condition `json:"conditions"`
	HostCount      int64             `json:"host_count"`
}

type RuleListResp struct {
	Items  []RuleResp `json:"items"`
	Total  int64      `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

type RuleStatsResp struct {
	EnabledRules    int64 `json:"enabled_rules"`
	TotalRules      int64 `json:"total_rules"`
	RuleHosts       int64 `json:"rule_hosts"`
	ManualHosts     int64 `json:"manual_hosts"`
	UnassignedHosts int64 `json:"unassigned_hosts"`
}

type PreviewItem struct {
	DeviceID            uint              `json:"device_id"`
	InstanceName        string            `json:"instance_name"`
	PrivateIP           string            `json:"private_ip"`
	CurrentDepartmentID uint              `json:"current_department_id"`
	DepartmentID        uint              `json:"department_id"`
	DepartmentName      string            `json:"department_name"`
	RuleID              uint              `json:"rule_id"`
	RuleName            string            `json:"rule_name"`
	MatchConditions     []model.Condition `json:"match_conditions"`
}

type PreviewResp struct {
	TotalUnassigned int64         `json:"total_unassigned"`
	MatchedTotal    int64         `json:"matched_total"`
	ClearedTotal    int64         `json:"cleared_total"`
	Items           []PreviewItem `json:"items"`
	Limit           int           `json:"limit"`
	Offset          int           `json:"offset"`
}

type ExecuteResp struct {
	Assigned int64 `json:"assigned"`
	Cleared  int64 `json:"cleared"`
}

type DeleteResp struct {
	Affected int64 `json:"affected"`
}

// Check 校验新增/编辑规则请求。
func (r *SaveRuleReq) Check() []error {
	var errs []error
	name := strings.TrimSpace(r.Name)
	if name == "" {
		errs = append(errs, fmt.Errorf("规则名称不能为空"))
	} else if len([]rune(name)) > MaxRuleNameLen {
		errs = append(errs, fmt.Errorf("规则名称长度不能超过%d个字符", MaxRuleNameLen))
	}
	if r.DepartmentID == 0 {
		errs = append(errs, fmt.Errorf("请选择目标部门"))
	}
	if len(r.Conditions) == 0 {
		errs = append(errs, fmt.Errorf("至少需要一个匹配条件"))
	}
	if len(r.Conditions) > MaxConditions {
		errs = append(errs, fmt.Errorf("匹配条件不能超过%d个", MaxConditions))
	}
	for index, condition := range r.Conditions {
		operator := strings.TrimSpace(condition.Operator)
		if !tools.IsValidOperator(operator) {
			errs = append(errs, fmt.Errorf("第%d个条件的运算符不受支持", index+1))
			continue
		}
		if len([]rune(condition.Value)) > MaxConditionValueLen {
			errs = append(errs, fmt.Errorf("第%d个条件的值不能超过%d个字符", index+1, MaxConditionValueLen))
			continue
		}
		if operator == tools.OperatorRegex {
			if strings.TrimSpace(condition.Value) == "" {
				errs = append(errs, fmt.Errorf("第%d个条件的正则表达式不能为空", index+1))
			} else if _, err := tools.CompileConditions([]model.Condition{condition}); err != nil {
				errs = append(errs, fmt.Errorf("第%d个条件的正则表达式格式错误", index+1))
			}
			continue
		}
		values := tools.NormalizeValues(condition.Value)
		if len(values) == 0 {
			errs = append(errs, fmt.Errorf("第%d个条件的值不能为空", index+1))
			continue
		}
		if len(values) > MaxValuesPerCondition {
			errs = append(errs, fmt.Errorf("第%d个条件的值不能超过%d个", index+1, MaxValuesPerCondition))
		}
		if tools.IsIPOperator(operator) {
			if _, err := tools.CompileConditions([]model.Condition{condition}); err != nil {
				// Keep the parser's detail so users can identify the invalid item in
				// a comma-separated list of IPs and CIDR prefixes.
				errs = append(errs, err)
			}
		}
	}
	return errs
}

// NormalizedConditions 返回去掉首尾空格后的条件，落库前统一格式。
func (r *SaveRuleReq) NormalizedConditions() []model.Condition {
	conditions := make([]model.Condition, 0, len(r.Conditions))
	for _, condition := range r.Conditions {
		conditions = append(conditions, model.Condition{
			Operator: strings.TrimSpace(condition.Operator),
			Value:    strings.TrimSpace(condition.Value),
		})
	}
	return conditions
}
