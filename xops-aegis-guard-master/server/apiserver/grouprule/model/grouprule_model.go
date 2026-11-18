package model

import basemodel "xops-aegis-guard/server/apiserver/base/model"

// Condition 是分组规则中的一条主机名或内网 IP 匹配条件。
// Value 允许用中英文逗号分隔多个值，正则匹配时整个 Value 作为一个表达式。
// ip_in 和 ip_not_in 的值为单个 IP 或 CIDR 网段。
type Condition struct {
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// GroupRule 主机自动分组规则。规则按 SortOrder 升序执行，命中第一条即停止。
type GroupRule struct {
	basemodel.BaseModel
	Name         string         `json:"name" gorm:"column:name;type:varchar(64);not null;comment:规则名称"`
	DepartmentID uint           `json:"department_id" gorm:"column:department_id;type:bigint;not null;index;comment:目标部门ID"`
	Enabled      bool           `json:"enabled" gorm:"column:enabled;type:boolean;not null;default:true;comment:是否启用"`
	SortOrder    int            `json:"sort_order" gorm:"column:sort_order;type:int;not null;default:0;index;comment:执行顺序，越小越先执行"`
	Conditions   basemodel.JSON `json:"conditions" gorm:"column:conditions;type:json;comment:匹配条件列表"`
}

// TableName 分组规则表名
func (GroupRule) TableName() string {
	return "group_rules"
}
