package model

import basemodel "xops-aegis-guard/server/apiserver/base/model"

type Department struct {
	basemodel.BaseModel
	ParentID   uint                 `json:"parent_id" gorm:"column:parent_id;type:bigint;index;comment:父部门ID"`
	Name       string               `json:"name" gorm:"column:name;type:varchar(128);not null;comment:部门名称"`
	Code       string               `json:"code" gorm:"column:code;type:varchar(64);not null;uniqueIndex;comment:部门编码"`
	RobotID    int                  `json:"robot_id" gorm:"column:robot_id;type:int;not null;default:0;comment:告警机器人ID"`
	Enabled    bool                 `json:"enabled" gorm:"column:enabled;type:boolean;not null;default:true;comment:是否启用"`
	DutyUserID basemodel.StringJSON `json:"duty_user_id" gorm:"column:duty_user_id;type:json;comment:群消息艾特的值班人员"`
	MemberIDs  basemodel.JSON       `json:"member_ids" gorm:"column:member_ids;type:json;comment:可查看部门告警的数字用户ID列表"`
}

// TableName 部门表名
func (Department) TableName() string {
	return "departments"
}
