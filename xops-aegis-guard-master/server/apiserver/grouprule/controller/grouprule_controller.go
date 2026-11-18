package controller

import (
	"strconv"

	"xops-aegis-guard/pkg/middleware"
	"xops-aegis-guard/pkg/mistake"
	"xops-aegis-guard/server/apiserver/grouprule/service"
	"xops-aegis-guard/server/apiserver/grouprule/val"
	"xops-aegis-guard/util/httper"

	"github.com/gin-gonic/gin"
)

func CreateRule(ctx *gin.Context) {
	req := &val.SaveRuleReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	row, err := service.Create(req)
	httper.HandleResponse(ctx, err, row)
}

func UpdateRule(ctx *gin.Context) {
	req := &val.SaveRuleReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	row, err := service.Update(req)
	httper.HandleResponse(ctx, err, row)
}

func DeleteRule(ctx *gin.Context) {
	req := &val.DeleteRuleReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	result, err := service.Delete(req)
	httper.HandleResponse(ctx, err, result)
}

func GetRule(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	req := &val.GetRuleReq{ID: id}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	row, err := service.Get(req)
	if service.IsNotFound(err) {
		httper.HandleResponse(ctx, mistake.NewReqErr("规则不存在"), nil)
		return
	}
	httper.HandleResponse(ctx, err, row)
}

func ListRules(ctx *gin.Context) {
	req := &val.ListRuleReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	rows, total, limit, offset, err := service.List(req)
	httper.HandleResponse(ctx, err, val.RuleListResp{Items: rows, Total: total, Limit: limit, Offset: offset})
}

func ToggleRule(ctx *gin.Context) {
	req := &val.ToggleRuleReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	err := service.SetEnabled(req)
	httper.HandleResponse(ctx, err, map[string]interface{}{"status": "updated"})
}

func ReorderRule(ctx *gin.Context) {
	req := &val.ReorderRuleReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	err := service.Reorder(req)
	httper.HandleResponse(ctx, err, map[string]interface{}{"status": "updated"})
}

func GetRuleStats(ctx *gin.Context) {
	req := &val.RuleStatsReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	row, err := service.Stats(req)
	httper.HandleResponse(ctx, err, row)
}

func PreviewRules(ctx *gin.Context) {
	req := &val.PreviewRuleReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	if httper.BindAndCheck(ctx, req) {
		return
	}
	result, err := service.Preview(req)
	httper.HandleResponse(ctx, err, result)
}

func ExecuteRules(ctx *gin.Context) {
	req := &val.ExecuteRuleReq{}
	req.AuthUser, _ = middleware.GetUserFromContext(ctx)
	result, err := service.Execute(req)
	httper.HandleResponse(ctx, err, result)
}

func parseID(ctx *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || value == 0 {
		httper.HandleResponse(ctx, mistake.NewReqErr("invalid id"), nil)
		return 0, false
	}
	return uint(value), true
}
