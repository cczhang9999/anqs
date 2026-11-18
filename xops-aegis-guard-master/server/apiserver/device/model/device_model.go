package model

import (
	"time"

	basemodel "xops-aegis-guard/server/apiserver/base/model"
)

// 设备归属来源。空字符串表示未分配。
const (
	AssignSourceRule   = "rule"
	AssignSourceManual = "manual"
)

type Device struct {
	basemodel.BaseModel
	Provider     string `json:"provider" gorm:"column:provider;type:varchar(32);not null;uniqueIndex:uk_device_provider_account_uuid,priority:1;comment:设备来源"`
	AccountType  string `json:"account_type" gorm:"column:account_type;type:varchar(16);not null;uniqueIndex:uk_device_provider_account_uuid,priority:2;comment:账号类型"`
	AliyunUUID   string `json:"aliyun_uuid" gorm:"column:aliyun_uuid;type:varchar(128);not null;uniqueIndex:uk_device_provider_account_uuid,priority:3;comment:云厂商设备UUID"`
	InstanceID   string `json:"instance_id" gorm:"column:instance_id;type:varchar(128);index;comment:实例ID"`
	InstanceName string `json:"instance_name" gorm:"column:instance_name;type:varchar(255);index;comment:实例名称"`
	PrivateIP    string `json:"private_ip" gorm:"column:private_ip;type:varchar(64);index;comment:内网IP"`
	OSName       string `json:"os_name" gorm:"column:os_name;type:varchar(128);comment:操作系统"`
	DepartmentID uint   `json:"department_id" gorm:"column:department_id;type:bigint;index;comment:部门ID"`
	// 归属来源：rule=规则自动归属、manual=人工分配、空=未分配。
	// 规则归属会同时保存规则 ID 和规则名称快照，规则被删除后仍能展示来源。
	AssignSource   string     `json:"assign_source" gorm:"column:assign_source;type:varchar(16);not null;default:'';index;comment:归属来源（rule/manual/空）"`
	AssignRuleID   uint       `json:"assign_rule_id" gorm:"column:assign_rule_id;type:bigint;not null;default:0;index;comment:命中的分组规则ID"`
	AssignRuleName string     `json:"assign_rule_name" gorm:"column:assign_rule_name;type:varchar(64);not null;default:'';comment:命中的分组规则名称快照"`
	AssignOperator string     `json:"assign_operator" gorm:"column:assign_operator;type:varchar(128);not null;default:'';comment:人工分配操作人"`
	AssignedAt     *time.Time `json:"assigned_at" gorm:"column:assigned_at;type:timestamp;comment:归属时间"`
}

// DeviceSimple 是设备筛选和关联场景使用的轻量设备信息。
type DeviceSimple struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	PrivateIP    string `json:"private_ip"`
	DepartmentID uint   `json:"department_id"`
}

// TableName 设备表名
func (Device) TableName() string {
	return "devices"
}
