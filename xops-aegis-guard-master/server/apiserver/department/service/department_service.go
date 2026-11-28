package service

import (
	"fmt"
	"strings"
	"time"

	"strconv"
	"xops-aegis-guard/pkg/mistake"
	"xops-aegis-guard/server/apiserver/department/dao"
	"xops-aegis-guard/server/apiserver/department/model"
	"xops-aegis-guard/server/apiserver/department/val"
	groupruledao "xops-aegis-guard/server/apiserver/grouprule/dao"
	"xops-aegis-guard/util/pager"

	"github.com/jinzhu/copier"
)

func IsNotFound(err error) bool { return dao.IsNotFound(err) }

func Create(req *val.SaveDepartmentReq) error {
	row := &model.Department{}
	if err := copier.Copy(row, &req.Department); err != nil {
		return err
	}
	// 编码不是业务配置项，兼容数据库唯一索引即可；由服务端根据名称生成。
	if strings.TrimSpace(row.Code) == "" {
		row.Code = fmt.Sprintf("dept-%d", time.Now().UnixNano())
	}
	return dao.Save(row)
}

func List(req *val.GetDepartmentsReq) ([]model.Department, error) { return dao.List() }

func ListByMember(userID int) ([]model.Department, error) { return dao.ListByMember(userID) }

func ListPage(req *val.ListDepartmentReq) ([]model.Department, int64, int, int, error) {
	limit, offset := pager.Parse(strconv.Itoa(req.Limit), strconv.Itoa(req.Offset))
	rows, total, err := dao.ListPage(limit, offset)
	return rows, total, limit, offset, err
}

func GetFallbackRobotID(req *val.GetDepartmentFallbackReq) (int, error) {
	return dao.GetFallbackRobotID()
}

func SaveFallbackRobotID(req *val.SaveDepartmentFallbackReq) error {
	return dao.SaveFallbackRobotID(req.RobotID)
}

func Get(req *val.GetDepartmentReq) (*model.Department, error) { return dao.Get(req.ID) }

func Update(req *val.SaveDepartmentReq) (*model.Department, error) {
	existing, err := dao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	row := &model.Department{}
	if err := copier.Copy(row, &req.Department); err != nil {
		return nil, err
	}
	row.ID = req.ID
	if strings.TrimSpace(row.Code) == "" {
		row.Code = existing.Code
	}
	if err := dao.Save(row); err != nil {
		return nil, err
	}
	return row, nil
}

func Delete(req *val.DeleteDepartmentReq) error {
	if _, err := dao.Get(req.ID); err != nil {
		return err
	}
	// 仍有分组规则指向该部门时禁止删除，避免规则指向不存在的部门。
	rules, err := groupruledao.CountByDepartment(req.ID)
	if err != nil {
		return err
	}
	if rules > 0 {
		return mistake.NewReqErr(fmt.Sprintf("该部门下存在 %d 条分组规则，请先删除或调整规则", rules))
	}
	return dao.Delete(req.ID)
}
