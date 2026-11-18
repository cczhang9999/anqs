package val

import (
	"fmt"
	"strings"
	"time"

	"xops-aegis-guard/pkg/user"
	basemodel "xops-aegis-guard/server/apiserver/base/model"
	"xops-aegis-guard/server/apiserver/device/model"
)

// SaveDeviceReq 新增或修改设备请求。
type SaveDeviceReq struct {
	AuthUser *user.AuthUser `json:"-"`
	model.Device
}

type ListDeviceReq struct {
	AuthUser     *user.AuthUser `json:"-"`
	Limit        int            `validate:"omitempty,min=0,max=200" comment:"每页数量" form:"limit"`
	Offset       int            `validate:"omitempty,min=0" comment:"偏移量" form:"offset"`
	Keyword      string         `validate:"omitempty,lte=128" comment:"搜索关键字" form:"keyword"`
	IP           string         `validate:"omitempty,lte=64" comment:"IP" form:"ip"`
	DepartmentID *uint          `validate:"omitempty,min=0" comment:"部门ID" form:"department_id"`
	// Source 按归属来源筛选：rule=规则、manual=手动、unassigned=未分配，空为全部。
	Source string `validate:"omitempty,oneof=rule manual unassigned" comment:"归属来源" form:"source"`
}

type AssignDepartmentReq struct {
	AuthUser     *user.AuthUser `json:"-"`
	ID           uint           `validate:"required,min=1" json:"id" form:"id" binding:"required" comment:"设备ID"`
	DepartmentID uint           `validate:"min=0" json:"department_id" form:"department_id" comment:"部门ID，0 表示未分组"`
}

type BatchAssignDepartmentReq struct {
	AuthUser     *user.AuthUser `json:"-"`
	IDs          []uint         `validate:"required,min=1,max=1000,dive,required,min=1" json:"ids" binding:"required" comment:"设备ID列表"`
	DepartmentID uint           `validate:"min=0" json:"department_id" comment:"部门ID，0 表示未分组"`
}

type DeleteDeviceReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `validate:"required,min=1" json:"id" form:"id" binding:"required" comment:"设备ID"`
}

type GetDevicesReq struct {
	AuthUser *user.AuthUser `json:"-"`
}
type GetSimpleDevicesReq struct {
	AuthUser *user.AuthUser `json:"-"`
}
type GetDeviceStatsReq struct {
	AuthUser *user.AuthUser `json:"-"`
}
type GetDeviceReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `json:"-"`
}
type SyncDevicesReq struct {
	AuthUser *user.AuthUser `json:"-"`
}

type DeviceListResp struct {
	Items  []DeviceResp `json:"items"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// DeviceResp 是设备接口的公开响应。云厂商设备 UUID 和实例 ID 仅供
// 服务内部同步及告警关联使用，不通过设备 API 返回。
type DeviceResp struct {
	basemodel.TimeModel
	Provider     string `json:"provider"`
	AccountType  string `json:"account_type"`
	InstanceName string `json:"instance_name"`
	PrivateIP    string `json:"private_ip"`
	OSName       string `json:"os_name"`
	DepartmentID uint   `json:"department_id"`
	// 归属来源：rule=规则、manual=手动、空=未分配；规则来源附带规则名称快照。
	AssignSource      string     `json:"assign_source"`
	AssignRuleID      uint       `json:"assign_rule_id"`
	AssignRuleName    string     `json:"assign_rule_name"`
	AssignRuleDeleted bool       `json:"assign_rule_deleted"`
	AssignOperator    string     `json:"assign_operator"`
	AssignedAt        *time.Time `json:"assigned_at"`
}

type DeviceStatsResp struct {
	Total      int64 `json:"total"`
	Assigned   int64 `json:"assigned"`
	Unassigned int64 `json:"unassigned"`
}

type DeviceSimpleListResp struct {
	Items []model.DeviceSimple `json:"items"`
	Total int64                `json:"total"`
}

// Check 校验设备新增/修改请求中的字符串长度。
// 设备同步产生的数据不会经过 HTTP 请求，因此不会受此校验影响。
func (r *SaveDeviceReq) Check() []error {
	checks := []struct {
		name  string
		value string
		limit int
	}{
		{"实例ID", r.InstanceID, 128},
		{"实例名称", r.InstanceName, 255},
		{"内网IP", r.PrivateIP, 64},
		{"操作系统", r.OSName, 128},
	}
	var errs []error
	for _, item := range checks {
		if len([]rune(strings.TrimSpace(item.value))) > item.limit {
			errs = append(errs, fmt.Errorf("%s长度不能超过%d个字符", item.name, item.limit))
		}
	}
	return errs
}
