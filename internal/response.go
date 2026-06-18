package internal

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一 JSON 响应格式。
// 简化自 Java Response.java，code + info + data 三字段信封。
type Response struct {
	Code string `json:"code"`
	Info string `json:"info"`
	Data any    `json:"data,omitempty"`
}

// Success 成功响应。
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code: CodeSuccess,
		Info: "成功",
		Data: data,
	})
}

// Fail 业务失败响应（HTTP 200 + 业务错误码）。
// handler 层统一用这个返回业务错误，不区分 HTTP 状态码。
func Fail(c *gin.Context, code string) {
	c.JSON(http.StatusOK, Response{
		Code: code,
		Info: Message(code),
	})
}

// FailWithMsg 业务失败响应（自定义消息）。
func FailWithMsg(c *gin.Context, code string, msg string) {
	c.JSON(http.StatusOK, Response{
		Code: code,
		Info: msg,
	})
}

// FailHTTP HTTP 层错误（4xx/5xx），不经过业务错误码体系。
func FailHTTP(c *gin.Context, status int, msg string) {
	c.JSON(status, Response{
		Code: CodeUnknownErr,
		Info: msg,
	})
}
