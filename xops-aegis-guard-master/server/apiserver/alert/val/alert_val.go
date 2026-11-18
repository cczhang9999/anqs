package val

import (
	"fmt"
	"strings"
	"time"

	"xops-aegis-guard/pkg/user"
	"xops-aegis-guard/server/apiserver/alert/model"
)

type SaveAlertReq struct {
	AuthUser *user.AuthUser `json:"-"`
	model.SecurityAlert
}

type ListAlertReq struct {
	AuthUser     *user.AuthUser `json:"-"`
	Source       string         `validate:"omitempty,lte=32" comment:"告警来源" form:"source"`
	AlertType    string         `validate:"omitempty,oneof=suspicious_event custom_incident core_file honeypot" comment:"告警类型" form:"alert_type"`
	Status       string         `validate:"omitempty,oneof=firing confirmed stale" comment:"告警状态" form:"status"`
	Severity     string         `validate:"omitempty,oneof=serious high suspicious medium low info unknown" comment:"告警等级" form:"severity"`
	Keyword      string         `validate:"omitempty,lte=255" comment:"搜索关键字" form:"keyword"`
	Host         string         `validate:"omitempty,lte=255" comment:"实例名称" form:"host"`
	IP           string         `validate:"omitempty,lte=64" comment:"IP" form:"ip"`
	From         string         `validate:"omitempty,len=10" comment:"开始日期" form:"from"`
	To           string         `validate:"omitempty,len=10" comment:"结束日期" form:"to"`
	DepartmentID uint           `validate:"omitempty,min=0" comment:"部门ID" form:"department_id"`
	DeviceID     uint           `validate:"omitempty,min=0" comment:"设备ID" form:"device_id"`
	Limit        int            `validate:"omitempty,min=0,max=200" comment:"每页数量" form:"limit"`
	Offset       int            `validate:"omitempty,min=0" comment:"偏移量" form:"offset"`
}

type ConfirmAlertReq struct {
	AuthUser     *user.AuthUser `json:"-"`
	ID           uint           `validate:"required,min=1" json:"id" form:"id" binding:"required" comment:"告警ID"`
	OccurrenceID uint           `json:"occurrence_id" form:"occurrence_id"`
	Note         string         `validate:"required,min=5,lte=1000" json:"note" form:"note" binding:"required" comment:"确认备注，至少 5 个字符"`
}

// Batch confirmation is one submission; each aggregate counts it once.
type ConfirmAlertsReq struct {
	AuthUser *user.AuthUser `json:"-"`
	IDs      []uint         `json:"ids" validate:"required,min=1,max=200,dive,min=1"`
	Note     string         `json:"note" validate:"required,min=5,lte=1000"`
}

type GetAlertsReq struct {
	AuthUser *user.AuthUser `json:"-"`
}
type GetAlertStatsReq struct {
	AuthUser *user.AuthUser `json:"-"`
}
type GetAlertReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `json:"-"`
}
type ListAlertHistoryReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `json:"-"`
}
type CollectAlertsReq struct {
	AuthUser *user.AuthUser `json:"-"`
}

type AlertListResp struct {
	Items  []AlertListItem `json:"items"`
	Total  int64           `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// AlertListItem 是列表使用的轻量响应，不携带原始 LastPayload，仅返回已脱敏的 LastMessage。
// FirstSeenAt 由 CreatedAt 映射，兼容前端“首次接收时间”的展示语义。
type AlertListItem struct {
	ID                uint       `json:"id"`
	CreatedAt         *time.Time `json:"created_at"`
	UpdatedAt         *time.Time `json:"updated_at"`
	Source            string     `json:"source"`
	ProductType       string     `json:"product_type"`
	AlertType         string     `json:"alert_type"`
	ProviderAlertID   string     `json:"provider_alert_id"`
	Title             string     `json:"title"`
	Severity          string     `json:"severity"`
	Status            string     `json:"status"`
	DeviceID          uint       `json:"device_id"`
	DeviceName        string     `json:"device_name"`
	DevicePrivateIP   string     `json:"device_private_ip"`
	DepartmentID      uint       `json:"department_id"`
	DepartmentName    string     `json:"department_name"`
	LastMessage       string     `json:"last_message"`
	FirstSeenAt       *time.Time `json:"first_seen_at"`
	FirstProviderTime *time.Time `json:"first_provider_time"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	LastProviderTime  *time.Time `json:"last_provider_time"`
	OccurrenceCount   int64      `json:"occurrence_count"`
	ConfirmedBy       string     `json:"confirmed_by"`
	ConfirmedAt       *time.Time `json:"confirmed_at"`
	ConfirmationCount int64      `json:"confirmation_count"`
	ConfirmedCount    int64      `json:"confirmed_count"`
	PendingCount      int64      `json:"pending_count"`
	ConfirmNote       string     `json:"confirm_note"`
}

// AlertDetailResp 是详情和确认接口的响应，完整 Payload 仅在按 ID 查询时返回。
type AlertDetailResp struct {
	ID                uint       `json:"id"`
	CreatedAt         *time.Time `json:"created_at"`
	UpdatedAt         *time.Time `json:"updated_at"`
	Source            string     `json:"source"`
	ProductType       string     `json:"product_type"`
	AlertType         string     `json:"alert_type"`
	ProviderAlertID   string     `json:"provider_alert_id"`
	Title             string     `json:"title"`
	Severity          string     `json:"severity"`
	Status            string     `json:"status"`
	DeviceID          uint       `json:"device_id"`
	FirstSeenAt       *time.Time `json:"first_seen_at"`
	FirstProviderTime *time.Time `json:"first_provider_time"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	LastProviderTime  *time.Time `json:"last_provider_time"`
	OccurrenceCount   int64      `json:"occurrence_count"`
	ConfirmNote       string     `json:"confirm_note"`
	ConfirmedBy       string     `json:"confirmed_by"`
	ConfirmedAt       *time.Time `json:"confirmed_at"`
	LastPayload       string     `json:"last_payload"`
	ConfirmationCount int64      `json:"confirmation_count"`
	ConfirmedCount    int64      `json:"confirmed_count"`
	PendingCount      int64      `json:"pending_count"`
}

type AlertHistoryResp struct {
	Items []AlertHistoryItem `json:"items"`
}

// AlertHistoryItem 是历史接口的时间线返回结构。
type AlertHistoryItem struct {
	ID          uint       `json:"id"`
	AlertID     uint       `json:"alert_id"`
	Title       string     `json:"title"`
	DeviceID    uint       `json:"device_id"`
	Severity    string     `json:"severity"`
	OccurredAt  string     `json:"occurred_at"`
	Snapshot    bool       `json:"snapshot,omitempty"`
	Status      string     `json:"status"`
	ConfirmedBy string     `json:"confirmed_by"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
	ConfirmNote string     `json:"confirm_note"`
}

type AlertStatsResp struct {
	Total     int64            `json:"total"`
	Pending   int64            `json:"pending"`
	Confirmed int64            `json:"confirmed"`
	Today     int64            `json:"today"`
	Severity  map[string]int64 `json:"severity"`
}

// Check 校验告警新增/修改请求。采集链路直接创建 model，不经过 HTTP 校验。
func (r *SaveAlertReq) Check() []error {
	var errs []error
	checks := []struct {
		name  string
		value string
		limit int
	}{
		{"告警来源", r.Source, 32},
		{"产品类型", r.ProductType, 16},
		{"告警类型", r.AlertType, 64},
		{"云厂商告警ID", r.ProviderAlertID, 255},
		{"告警指纹", r.Fingerprint, 64},
		{"告警标题", r.Title, 512},
		{"告警等级", r.Severity, 32},
		{"告警状态", r.Status, 16},
	}
	for _, item := range checks {
		if len([]rune(strings.TrimSpace(item.value))) > item.limit {
			errs = append(errs, fmt.Errorf("%s长度不能超过%d个字符", item.name, item.limit))
		}
	}
	if strings.TrimSpace(r.Source) == "" {
		errs = append(errs, fmt.Errorf("告警来源不能为空"))
	}
	if strings.TrimSpace(r.ProductType) == "" {
		errs = append(errs, fmt.Errorf("产品类型不能为空"))
	}
	if strings.TrimSpace(r.AlertType) == "" {
		errs = append(errs, fmt.Errorf("告警类型不能为空"))
	}
	if strings.TrimSpace(r.Fingerprint) == "" {
		errs = append(errs, fmt.Errorf("告警指纹不能为空"))
	}
	if r.Status != "" && !isAllowed(r.Status, "firing", "confirmed", "stale") {
		errs = append(errs, fmt.Errorf("告警状态不合法"))
	}
	if r.Severity != "" && !isAllowed(r.Severity, "serious", "high", "suspicious", "medium", "low", "info", "unknown") {
		errs = append(errs, fmt.Errorf("告警等级不合法"))
	}
	if r.OccurrenceCount < 0 {
		errs = append(errs, fmt.Errorf("发生次数不能小于0"))
	}
	return errs
}

// Check 校验告警查询中的日期值和日期范围。
func (r *ListAlertReq) Check() []error {
	var errs []error
	var from, to time.Time
	if r.From != "" {
		var err error
		from, err = time.Parse("2006-01-02", r.From)
		if err != nil {
			errs = append(errs, fmt.Errorf("开始日期格式必须为YYYY-MM-DD"))
		}
	}
	if r.To != "" {
		var err error
		to, err = time.Parse("2006-01-02", r.To)
		if err != nil {
			errs = append(errs, fmt.Errorf("结束日期格式必须为YYYY-MM-DD"))
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		errs = append(errs, fmt.Errorf("开始日期不能晚于结束日期"))
	}
	return errs
}

func isAllowed(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func (r *ConfirmAlertReq) Check() []error  { return checkConfirmationNote(r.Note) }
func (r *ConfirmAlertsReq) Check() []error { return checkConfirmationNote(r.Note) }
func checkConfirmationNote(note string) []error {
	if n := len([]rune(strings.TrimSpace(note))); n < 5 || n > 1000 {
		return []error{fmt.Errorf("确认备注须为 5 至 1000 个字符")}
	}
	return nil
}
