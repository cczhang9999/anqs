package httper

import (
	"net/http"
	"xops-aegis-guard/logger"
	"xops-aegis-guard/pkg/mistake"
	"xops-aegis-guard/util/str"

	"github.com/gin-gonic/gin"

	"xops-aegis-guard/pkg/validator"
)

// HandleResponse 统一处理异常，统一处理日志，统一处理返回
func HandleResponse(c *gin.Context, err error, data interface{}) {
	// 如果没有错误，就是正常请求
	if err == nil {
		c.JSON(http.StatusOK, SucceedRespBodyAndData("success", DumpContent(data)))
		return
	}

	// 处理请求参数异常
	if reqErr, ok := err.(*mistake.ReqErr); ok {
		logger.Debug(reqErr.Message)
		c.JSON(http.StatusBadRequest, ErrRespBody(reqErr.Message))
		return
	}
	logger.Debug(err.Error())
	c.JSON(http.StatusInternalServerError, ErrRespBody(err.Error()))
	return
}

// BindAndCheck 绑定参数并验证参数
// true: 确实存在问题，会返回400错误
// false: 不存在问题，验证通过
func BindAndCheck(ctx *gin.Context, data interface{}) bool {
	// 参数映射
	if err := ctx.ShouldBind(data); err != nil {
		// 当入参格式不正确的会出现，比如int传递为string
		logger.Errorf("http_handle BindAndCheck fail, %s", err.Error())
		HandleResponse(ctx, mistake.NewReqErr("请求参数绑定异常"), nil)
		return true
	}
	// 去除结构体内部数据前后的空格
	str.TrimStruct(data)
	// 验证参数
	if err := validator.GlobalValidate.Check(data); err != nil {
		HandleResponse(ctx, mistake.NewReqErr(err.Error()), nil)
		return true
	}

	return false
}
