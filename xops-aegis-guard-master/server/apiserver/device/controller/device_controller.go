package controller

import (
	"strconv"

	"xops-aegis-guard/pkg/middleware"
	"xops-aegis-guard/pkg/mistake"
	"xops-aegis-guard/server/apiserver/device/service"
	"xops-aegis-guard/server/apiserver/device/val"
	"xops-aegis-guard/util/httper"

	"github.com/gin-gonic/gin"
)

func CreateDevice(ctx *gin.Context) {
	req := &val.SaveDeviceReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	row, err := service.Create(req)
	httper.HandleResponse(ctx, err, row)
}

func ListDevices(ctx *gin.Context) {
	req := &val.ListDeviceReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	rows, total, limit, offset, err := service.List(req)
	httper.HandleResponse(ctx, err, val.DeviceListResp{Items: rows, Total: total, Limit: limit, Offset: offset})
}

func GetDevices(ctx *gin.Context) {
	req := &val.GetDevicesReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	rows, err := service.All(req)
	httper.HandleResponse(ctx, err, val.DeviceListResp{Items: rows, Total: int64(len(rows))})
}

func GetSimpleDevices(ctx *gin.Context) {
	req := &val.GetSimpleDevicesReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	rows, err := service.Simple(req)
	httper.HandleResponse(ctx, err, val.DeviceSimpleListResp{Items: rows, Total: int64(len(rows))})
}

func GetDeviceStats(ctx *gin.Context) {
	req := &val.GetDeviceStatsReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	total, assigned, unassigned, err := service.Stats(req)
	httper.HandleResponse(ctx, err, val.DeviceStatsResp{Total: total, Assigned: assigned, Unassigned: unassigned})
}

func GetDevice(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	req := &val.GetDeviceReq{ID: id}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	row, err := service.Get(req)
	if service.IsNotFound(err) {
		httper.HandleResponse(ctx, mistake.NewReqErr("device not found"), nil)
		return
	}
	httper.HandleResponse(ctx, err, row)
}

func UpdateDevice(ctx *gin.Context) {
	req := &val.SaveDeviceReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	if req.ID == 0 {
		httper.HandleResponse(ctx, mistake.NewReqErr("invalid id"), nil)
		return
	}
	row, err := service.Update(req)
	httper.HandleResponse(ctx, err, row)
}

func DeleteDevice(ctx *gin.Context) {
	req := &val.DeleteDeviceReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	err := service.Delete(req)
	httper.HandleResponse(ctx, err, nil)
}

func AssignDeviceDepartment(ctx *gin.Context) {
	req := &val.AssignDepartmentReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	err := service.AssignDepartment(req)
	httper.HandleResponse(ctx, err, map[string]interface{}{"status": "assigned"})
}

func BatchAssignDeviceDepartment(ctx *gin.Context) {
	req := &val.BatchAssignDepartmentReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	err := service.BatchAssignDepartment(req)
	httper.HandleResponse(ctx, err, map[string]interface{}{"status": "assigned", "count": len(req.IDs)})
}

func SyncDevices(ctx *gin.Context) {
	req := &val.SyncDevicesReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	err := service.SyncDevices(req)
	httper.HandleResponse(ctx, err, map[string]interface{}{"status": "synced"})
}

func parseID(ctx *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || value == 0 {
		httper.HandleResponse(ctx, mistake.NewReqErr("invalid id"), nil)
		return 0, false
	}
	return uint(value), true
}
