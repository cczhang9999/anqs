package controller

import "github.com/gin-gonic/gin"

func RegisterDeviceRoute(r *gin.RouterGroup) {
	r.POST("/device", CreateDevice)
	r.PUT("/device", UpdateDevice)
	r.DELETE("/device", DeleteDevice)
	r.GET("/device/:id", GetDevice)
	r.GET("/devices", GetDevices)
	r.GET("/devicesimple", GetSimpleDevices)
	r.GET("/devicelist", ListDevices)
	r.GET("/devicestats", GetDeviceStats)
	r.PUT("/device-department", AssignDeviceDepartment)
	r.PUT("/device-departments", BatchAssignDeviceDepartment)
	r.POST("/device-sync", SyncDevices)
}
