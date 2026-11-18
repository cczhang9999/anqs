package router

import (
	"net/http"

	"xops-aegis-guard/config"
	"xops-aegis-guard/logger"
	"xops-aegis-guard/pkg/middleware"
	alertcontroller "xops-aegis-guard/server/apiserver/alert/controller"
	departmentcontroller "xops-aegis-guard/server/apiserver/department/controller"
	devicecontroller "xops-aegis-guard/server/apiserver/device/controller"
	grouprulecontroller "xops-aegis-guard/server/apiserver/grouprule/controller"

	"github.com/gin-gonic/gin"
)

// InitRouter 按照 xops-port-check 的方式集中注册 Gin 路由。
func InitRouter(conf *config.Config) error {
	if !conf.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", func(ctx *gin.Context) { ctx.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	v1 := r.Group("/v1")
	v1.Use(logRequest, middleware.AuthUser)
	registerRouter(v1)
	return r.Run(":" + conf.Server.HTTPPort)
}

func registerRouter(r *gin.RouterGroup) {
	r.GET("/ping", func(ctx *gin.Context) { ctx.String(http.StatusOK, "pong") })
	alertcontroller.RegisterAlertRoute(r)
	devicecontroller.RegisterDeviceRoute(r)
	departmentcontroller.RegisterDepartmentRoute(r)
	grouprulecontroller.RegisterGroupRuleRoute(r)
}

func logRequest(ctx *gin.Context) {
	logger.Infof("%s %s %s", ctx.Request.Method, ctx.Request.URL.String(), ctx.ClientIP())
	ctx.Next()
}
