package controller

import "github.com/gin-gonic/gin"

func RegisterAlertRoute(r *gin.RouterGroup) {
	r.POST("/alert", CreateAlert)
	r.PUT("/alert", UpdateAlert)
	r.GET("/alert/:id/history", ListAlertHistory)
	r.GET("/alert/:id", GetAlert)
	r.GET("/alerts", GetAlerts)
	r.GET("/alertstats", GetAlertStats)
	r.GET("/alertlist", ListAlerts)
	r.POST("/alert-confirm", ConfirmAlert)
	r.POST("/alerts-confirm", ConfirmAlerts)
	r.POST("/alert-collect", CollectAlerts)
}
