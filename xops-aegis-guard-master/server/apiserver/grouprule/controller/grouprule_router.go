package controller

import "github.com/gin-gonic/gin"

// RegisterGroupRuleRoute 注册分组规则相关接口。
func RegisterGroupRuleRoute(r *gin.RouterGroup) {
	r.POST("/group-rule", CreateRule)
	r.PUT("/group-rule", UpdateRule)
	r.DELETE("/group-rule", DeleteRule)
	r.GET("/group-rule/:id", GetRule)
	r.GET("/group-rules", ListRules)
	r.PUT("/group-rule-enabled", ToggleRule)
	r.PUT("/group-rule-order", ReorderRule)
	r.GET("/group-rulestats", GetRuleStats)
	r.GET("/group-rule-preview", PreviewRules)
	r.POST("/group-rule-execute", ExecuteRules)
}
