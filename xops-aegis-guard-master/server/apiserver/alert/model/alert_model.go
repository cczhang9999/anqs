package model

import (
	"time"
	basemodel "xops-aegis-guard/server/apiserver/base/model"
)

type SecurityAlert struct {
	basemodel.TimeModel
	Source               string     `json:"source" gorm:"column:source;type:varchar(32);not null;uniqueIndex:uk_alert_identity,priority:1;comment:告警来源"`
	ProductType          string     `json:"product_type" gorm:"column:product_type;type:varchar(16);not null;uniqueIndex:uk_alert_identity,priority:2;comment:产品类型"`
	AlertType            string     `json:"alert_type" gorm:"column:alert_type;type:varchar(64);not null;uniqueIndex:uk_alert_identity,priority:3;comment:告警类型"`
	ProviderAlertID      string     `json:"provider_alert_id" gorm:"column:provider_alert_id;type:varchar(255);index;comment:云厂商告警ID"`
	Fingerprint          string     `json:"fingerprint" gorm:"column:fingerprint;type:varchar(64);not null;uniqueIndex:uk_alert_identity,priority:4;comment:告警指纹"`
	Title                string     `json:"title" gorm:"column:title;type:varchar(512);index;comment:告警标题"`
	Severity             string     `json:"severity" gorm:"column:severity;type:varchar(32);index;comment:告警等级"`
	Status               string     `json:"status" gorm:"column:status;type:varchar(16);not null;default:firing;index;comment:告警状态"`
	DeviceID             uint       `json:"device_id" gorm:"column:device_id;type:bigint;index;comment:设备ID"`
	FirstProviderTime    *time.Time `json:"first_provider_time" gorm:"column:first_provider_time;type:timestamp;index;comment:云厂商首次发生时间"`
	LastSeenAt           time.Time  `json:"last_seen_at" gorm:"column:last_seen_at;type:timestamp;index;comment:最近发现时间"`
	LastProviderTime     *time.Time `json:"last_provider_time" gorm:"column:last_provider_time;type:timestamp;comment:云厂商最近时间"`
	OccurrenceCount      int64      `json:"occurrence_count" gorm:"column:occurrence_count;type:bigint;not null;default:1;comment:发生次数"`
	ConfirmationCount    int64      `json:"confirmation_count" gorm:"not null;default:0;comment:确认提交次数"`
	ConfirmedCount       int64      `json:"confirmed_count" gorm:"not null;default:0;comment:已确认明细数"`
	PendingCount         int64      `json:"pending_count" gorm:"not null;default:0;comment:待确认明细数"`
	ConfirmationMigrated bool       `json:"-" gorm:"not null;default:false;comment:明细确认迁移完成"`
	ConfirmNote          string     `json:"confirm_note" gorm:"column:confirm_note;type:varchar(1000);comment:确认信息"`
	ConfirmedByID        int        `json:"confirmed_by_id" gorm:"column:confirmed_by_id;type:int;not null;default:0;comment:确认人ID"`
	ConfirmedBy          string     `json:"confirmed_by" gorm:"column:confirmed_by;type:varchar(128);comment:确认人"`
	ConfirmedAt          *time.Time `json:"confirmed_at" gorm:"column:confirmed_at;type:timestamp;comment:确认时间"`
	LastPayload          string     `json:"last_payload" gorm:"column:last_payload;type:longtext;comment:最近一次告警原始数据"`
	LastMessage          string     `json:"last_message" gorm:"column:last_message;type:longtext;comment:最近一次告警消息"`
	ContentHash          string     `json:"-" gorm:"column:content_hash;type:char(64);not null;default:'';comment:告警实质内容指纹"`
}

// TableName 安全告警表名
func (SecurityAlert) TableName() string {
	return "security_alerts"
}

// AlertOccurrence 每次告警发生保存一行记录。
// CreateModel 提供 id、created_at，避免为高频发生记录维护无意义的 updated_at。
type AlertOccurrence struct {
	basemodel.CreateModel
	AlertID          uint       `json:"alert_id" gorm:"column:alert_id;type:bigint;not null;index;comment:聚合告警ID"`
	EventKey         []byte     `json:"-" gorm:"column:event_key;type:binary(32);comment:云端发生事件唯一键"`
	ConfirmBatchID   string     `json:"-" gorm:"type:varchar(64);not null;default:'';index;comment:确认提交批次"`
	ConfirmedByID    int        `json:"confirmed_by_id" gorm:"not null;default:0"`
	ConfirmedBy      string     `json:"confirmed_by" gorm:"type:varchar(128)"`
	ConfirmedAt      *time.Time `json:"confirmed_at" gorm:"type:timestamp"`
	ConfirmNote      string     `json:"confirm_note" gorm:"type:varchar(1000)"`
	Snapshot         bool       `json:"snapshot,omitempty" gorm:"not null;default:false"`
	RepresentedCount int64      `json:"-" gorm:"not null;default:1;comment:快照代表的历史发生数"`
	OccurredAt       time.Time  `json:"occurred_at" gorm:"column:occurred_at;type:timestamp;not null;index;comment:告警发生时间"`
}

// TableName 历史告警明细表名
func (AlertOccurrence) TableName() string {
	return "security_alert_occurrences"
}
