package controller

import "github.com/gin-gonic/gin"

func RegisterDepartmentRoute(r *gin.RouterGroup) {
	r.POST("/department", CreateDepartment)
	r.PUT("/department", UpdateDepartment)
	r.DELETE("/department", DeleteDepartment)
	r.GET("/department/:id", GetDepartment)
	r.GET("/departments", ListDepartments)
	r.GET("/member-departments", ListMemberDepartments)
	r.GET("/departmentlist", ListDepartmentsWithPage)
	r.GET("/department-fallback", GetDepartmentFallback)
	r.PUT("/department-fallback", SaveDepartmentFallback)
}
