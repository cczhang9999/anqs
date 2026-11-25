package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"xops-aegis-guard/logger"
	"xops-aegis-guard/pkg/aliyun"
	"xops-aegis-guard/pkg/user"
	departmentdao "xops-aegis-guard/server/apiserver/department/dao"
	"xops-aegis-guard/server/apiserver/device/dao"
	"xops-aegis-guard/server/apiserver/device/model"
	"xops-aegis-guard/server/apiserver/device/val"
	groupruledao "xops-aegis-guard/server/apiserver/grouprule/dao"
	groupruleservice "xops-aegis-guard/server/apiserver/grouprule/service"
	"xops-aegis-guard/util/pager"

	"github.com/jinzhu/copier"
)

type syncService struct {
	clients *aliyun.Clients
}

var syncInstance *syncService

func Init(clients *aliyun.Clients) {
	syncInstance = &syncService{clients: clients}
}

func IsNotFound(err error) bool { return dao.IsNotFound(err) }

func Create(req *val.SaveDeviceReq) (*val.DeviceResp, error) {
	row := &model.Device{}
	if err := copier.Copy(row, &req.Device); err != nil {
		return nil, err
	}
	// 新设备统一从未分组开始，只能通过分配接口加入部门。
	row.DepartmentID = 0
	row.AssignSource = ""
	row.AssignRuleID = 0
	row.AssignRuleName = ""
	row.AssignOperator = ""
	row.AssignedAt = nil
	if err := dao.Save(row); err != nil {
		return nil, err
	}
	return toDeviceResp(row)
}

func List(req *val.ListDeviceReq) ([]val.DeviceResp, int64, int, int, error) {
	limit, offset := pager.Parse(strconv.Itoa(req.Limit), strconv.Itoa(req.Offset))
	rows, total, err := dao.List(limit, offset, req.Keyword, req.IP, req.DepartmentID, req.Source)
	if err != nil {
		return nil, total, limit, offset, err
	}
	resp, err := toDeviceResps(rows)
	return resp, total, limit, offset, err
}

func Stats(req *val.GetDeviceStatsReq) (total, assigned, unassigned int64, err error) {
	return dao.Stats()
}

func All(req *val.GetDevicesReq) ([]val.DeviceResp, error) {
	rows, err := dao.All()
	if err != nil {
		return nil, err
	}
	return toDeviceResps(rows)
}

func Simple(req *val.GetSimpleDevicesReq) ([]model.DeviceSimple, error) { return dao.Simple() }

func Get(req *val.GetDeviceReq) (*val.DeviceResp, error) {
	row, err := dao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	return toDeviceResp(row)
}

func Update(req *val.SaveDeviceReq) (*val.DeviceResp, error) {
	existing, err := dao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	row := &model.Device{}
	if err := copier.Copy(row, &req.Device); err != nil {
		return nil, err
	}
	// 部门与归属来源只能通过分配接口设置，普通更新不能覆盖。
	row.ID = req.ID
	row.DepartmentID = existing.DepartmentID
	row.AssignSource = existing.AssignSource
	row.AssignRuleID = existing.AssignRuleID
	row.AssignRuleName = existing.AssignRuleName
	row.AssignOperator = existing.AssignOperator
	row.AssignedAt = existing.AssignedAt
	if err := dao.Save(row); err != nil {
		return nil, err
	}
	return toDeviceResp(row)
}

func Delete(req *val.DeleteDeviceReq) error {
	if _, err := dao.Get(req.ID); err != nil {
		return err
	}
	return dao.Delete(req.ID)
}

func AssignDepartment(req *val.AssignDepartmentReq) error {
	if _, err := dao.Get(req.ID); err != nil {
		return err
	}
	// 0 是未分组，允许人工将设备从部门移回未分组（交还给规则）。
	if req.DepartmentID == 0 {
		return dao.AssignDepartment(req.ID, 0, operatorName(req.AuthUser))
	}
	if _, err := departmentdao.Get(req.DepartmentID); err != nil {
		return err
	}
	return dao.AssignDepartment(req.ID, req.DepartmentID, operatorName(req.AuthUser))
}

func BatchAssignDepartment(req *val.BatchAssignDepartmentReq) error {
	if req.DepartmentID != 0 {
		if _, err := departmentdao.Get(req.DepartmentID); err != nil {
			return err
		}
	}
	return dao.BatchAssignDepartment(req.IDs, req.DepartmentID, operatorName(req.AuthUser))
}

// operatorName 记录人工分配的操作人，未登录时回落到内部调用。
func operatorName(authUser *user.AuthUser) string {
	if authUser == nil {
		return ""
	}
	name := strings.TrimSpace(authUser.Username)
	if name == "" {
		name = strings.TrimSpace(authUser.Email)
	}
	return name
}

func Sync() error {
	if syncInstance == nil || syncInstance.clients == nil {
		return errors.New("device service is not initialized")
	}
	devices, err := syncInstance.clients.PullDevices(1000, 0)
	if err != nil {
		return err
	}
	for _, item := range devices {
		row := &model.Device{Provider: "aliyun"}
		if err := copier.Copy(row, &item); err != nil {
			return err
		}
		row.AliyunUUID = item.UUID
		if err := dao.Upsert(row); err != nil {
			return err
		}
	}
	logger.Infof("aliyun devices synced: count=%d", len(devices))
	if err := groupruleservice.AssignUnassigned(); err != nil {
		return fmt.Errorf("apply group rules after device sync: %w", err)
	}
	return nil
}

// SyncDevices 是 HTTP 层使用的请求对象入口；Sync 保留给定时任务等内部调用。
func SyncDevices(req *val.SyncDevicesReq) error { return Sync() }

func toDeviceResp(row *model.Device) (*val.DeviceResp, error) {
	resp := &val.DeviceResp{}
	if err := copier.Copy(resp, row); err != nil {
		return nil, err
	}
	if resp.AssignSource == model.AssignSourceRule {
		existing, err := groupruledao.ExistingIDs()
		if err != nil {
			return nil, err
		}
		_, ok := existing[resp.AssignRuleID]
		resp.AssignRuleDeleted = !ok
	}
	return resp, nil
}

func toDeviceResps(rows []model.Device) ([]val.DeviceResp, error) {
	resp := make([]val.DeviceResp, 0, len(rows))
	if err := copier.Copy(&resp, &rows); err != nil {
		return nil, err
	}
	hasRule := false
	for _, item := range resp {
		if item.AssignSource == model.AssignSourceRule {
			hasRule = true
			break
		}
	}
	if !hasRule {
		return resp, nil
	}
	existing, err := groupruledao.ExistingIDs()
	if err != nil {
		return nil, err
	}
	for i := range resp {
		if resp[i].AssignSource != model.AssignSourceRule {
			continue
		}
		_, ok := existing[resp[i].AssignRuleID]
		resp[i].AssignRuleDeleted = !ok
	}
	return resp, nil
}
