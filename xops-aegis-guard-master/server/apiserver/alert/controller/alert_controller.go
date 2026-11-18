package controller

import (
	"strconv"

	"xops-aegis-guard/pkg/middleware"
	"xops-aegis-guard/pkg/mistake"
	"xops-aegis-guard/server/apiserver/alert/service"
	"xops-aegis-guard/server/apiserver/alert/val"
	"xops-aegis-guard/util/httper"

	"github.com/gin-gonic/gin"
)

func CreateAlert(ctx *gin.Context) {
	req := &val.SaveAlertReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	err := service.Create(req)
	httper.HandleResponse(ctx, err, req.SecurityAlert)
}

func ListAlerts(ctx *gin.Context) {
	req := &val.ListAlertReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	rows, total, limit, offset, err := service.List(req)
	httper.HandleResponse(ctx, err, val.AlertListResp{Items: rows, Total: total, Limit: limit, Offset: offset})
}

func GetAlerts(ctx *gin.Context) {
	req := &val.GetAlertsReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	rows, err := service.All(req)
	httper.HandleResponse(ctx, err, val.AlertListResp{Items: rows, Total: int64(len(rows))})
}

func GetAlertStats(ctx *gin.Context) {
	req := &val.GetAlertStatsReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	total, pending, confirmed, today, severity, err := service.Stats(req)
	httper.HandleResponse(ctx, err, val.AlertStatsResp{Total: total, Pending: pending, Confirmed: confirmed, Today: today, Severity: severity})
}

func GetAlert(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	req := &val.GetAlertReq{ID: id}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	row, err := service.Get(req)
	if service.IsNotFound(err) {
		httper.HandleResponse(ctx, mistake.NewReqErr("alert not found"), nil)
		return
	}
	httper.HandleResponse(ctx, err, row)
}

func ListAlertHistory(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	req := &val.ListAlertHistoryReq{ID: id}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	rows, err := service.ListHistory(req)
	if service.IsNotFound(err) {
		httper.HandleResponse(ctx, mistake.NewReqErr("alert not found"), nil)
		return
	}
	httper.HandleResponse(ctx, err, val.AlertHistoryResp{Items: rows})
}

func UpdateAlert(ctx *gin.Context) {
	req := &val.SaveAlertReq{}
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

func ConfirmAlert(ctx *gin.Context) {
	req := &val.ConfirmAlertReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	row, err := service.Confirm(req)
	if service.IsNotFound(err) {
		httper.HandleResponse(ctx, mistake.NewReqErr("alert not found"), nil)
		return
	}
	httper.HandleResponse(ctx, err, row)
}

func ConfirmAlerts(ctx *gin.Context) {
	req := &val.ConfirmAlertsReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	rows, err := service.ConfirmBatch(req)
	if service.IsNotFound(err) {
		httper.HandleResponse(ctx, mistake.NewReqErr("alert not found"), nil)
		return
	}
	httper.HandleResponse(ctx, err, map[string]interface{}{"items": rows})
}

func CollectAlerts(ctx *gin.Context) {
	req := &val.CollectAlertsReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	err := service.Collect(req)
	httper.HandleResponse(ctx, err, map[string]interface{}{"status": "collected"})
}

func parseID(ctx *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || value == 0 {
		httper.HandleResponse(ctx, mistake.NewReqErr("invalid id"), nil)
		return 0, false
	}
	return uint(value), true
}
