package val

import (
	"fmt"
	"strings"

	"xops-aegis-guard/pkg/user"
	"xops-aegis-guard/server/apiserver/department/model"
	jsonutil "xops-aegis-guard/util/json"
)

type SaveDepartmentReq struct {
	AuthUser *user.AuthUser `json:"-"`
	model.Department
}

type DeleteDepartmentReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `validate:"required,min=1" json:"id" form:"id" binding:"required" comment:"部门ID"`
}

type ListDepartmentReq struct {
	AuthUser *user.AuthUser `json:"-"`
	Limit    int            `validate:"omitempty,min=0,max=200" comment:"每页数量" form:"limit"`
	Offset   int            `validate:"omitempty,min=0" comment:"偏移量" form:"offset"`
}

type SaveDepartmentFallbackReq struct {
	AuthUser *user.AuthUser `json:"-"`
	RobotID  int            `validate:"min=0" comment:"机器人ID" json:"robot_id" form:"robot_id"`
}

type GetDepartmentsReq struct {
	AuthUser *user.AuthUser `json:"-"`
}
type GetDepartmentFallbackReq struct {
	AuthUser *user.AuthUser `json:"-"`
}
type GetDepartmentReq struct {
	AuthUser *user.AuthUser `json:"-"`
	ID       uint           `json:"-"`
}

type DepartmentListResp struct {
	Items []model.Department `json:"items"`
}

type DepartmentPageResp struct {
	Items  []model.Department `json:"items"`
	Total  int64              `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

type DepartmentFallbackResp struct {
	RobotID int `json:"robot_id"`
}

// Check 校验部门管理请求。
func (r *SaveDepartmentReq) Check() []error {
	var errs []error
	name := strings.TrimSpace(r.Name)
	if name == "" {
		errs = append(errs, fmt.Errorf("部门名称不能为空"))
	} else if len([]rune(name)) > 128 {
		errs = append(errs, fmt.Errorf("部门名称长度不能超过128个字符"))
	}
	if len([]rune(strings.TrimSpace(r.Code))) > 64 {
		errs = append(errs, fmt.Errorf("部门编码长度不能超过64个字符"))
	}
	if r.RobotID < 0 {
		errs = append(errs, fmt.Errorf("机器人ID不能小于0"))
	}
	if len(r.MemberIDs) > 0 && string(r.MemberIDs) != "null" {
		var memberIDs []int
		if err := jsonutil.Unmarshal([]byte(r.MemberIDs), &memberIDs); err != nil {
			errs = append(errs, fmt.Errorf("可见成员ID必须是数字数组"))
		} else {
			for _, id := range memberIDs {
				if id <= 0 {
					errs = append(errs, fmt.Errorf("可见成员ID必须大于0"))
					break
				}
			}
		}
	}
	return errs
}
