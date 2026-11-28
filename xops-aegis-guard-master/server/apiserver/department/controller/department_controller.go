package controller

import (
	"strconv"

	"xops-aegis-guard/pkg/middleware"
	"xops-aegis-guard/pkg/mistake"
	"xops-aegis-guard/server/apiserver/department/service"
	"xops-aegis-guard/server/apiserver/department/val"
	"xops-aegis-guard/util/httper"

	"github.com/gin-gonic/gin"
)

func CreateDepartment(ctx *gin.Context) {
	req := &val.SaveDepartmentReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	err := service.Create(req)
	httper.HandleResponse(ctx, err, req.Department)
}

func ListDepartments(ctx *gin.Context) {
	req := &val.GetDepartmentsReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	rows, err := service.List(req)
	httper.HandleResponse(ctx, err, val.DepartmentListResp{Items: rows})
}

func ListMemberDepartments(ctx *gin.Context) {
	rows, err := service.ListByMember(middleware.TokenUserID(ctx))
	httper.HandleResponse(ctx, err, val.DepartmentListResp{Items: rows})
}

func ListDepartmentsWithPage(ctx *gin.Context) {
	req := &val.ListDepartmentReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	rows, total, limit, offset, err := service.ListPage(req)
	httper.HandleResponse(ctx, err, val.DepartmentPageResp{Items: rows, Total: total, Limit: limit, Offset: offset})
}

func GetDepartmentFallback(ctx *gin.Context) {
	req := &val.GetDepartmentFallbackReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	robotID, err := service.GetFallbackRobotID(req)
	httper.HandleResponse(ctx, err, val.DepartmentFallbackResp{RobotID: robotID})
}

func SaveDepartmentFallback(ctx *gin.Context) {
	req := &val.SaveDepartmentFallbackReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	if req.RobotID < 0 {
		httper.HandleResponse(ctx, mistake.NewReqErr("invalid robot id"), nil)
		return
	}
	err := service.SaveFallbackRobotID(req)
	httper.HandleResponse(ctx, err, val.DepartmentFallbackResp{RobotID: req.RobotID})
}

func GetDepartment(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	req := &val.GetDepartmentReq{ID: id}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	row, err := service.Get(req)
	if service.IsNotFound(err) {
		httper.HandleResponse(ctx, mistake.NewReqErr("department not found"), nil)
		return
	}
	httper.HandleResponse(ctx, err, row)
}

func UpdateDepartment(ctx *gin.Context) {
	req := &val.SaveDepartmentReq{}
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

func DeleteDepartment(ctx *gin.Context) {
	req := &val.DeleteDepartmentReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	err := service.Delete(req)
	httper.HandleResponse(ctx, err, nil)
}

func parseID(ctx *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || value == 0 {
		httper.HandleResponse(ctx, mistake.NewReqErr("invalid id"), nil)
		return 0, false
	}
	return uint(value), true
}
