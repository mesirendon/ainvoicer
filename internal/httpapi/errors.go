package httpapi

import (
	"github.com/gin-gonic/gin"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeError(c *gin.Context, status int, code, message string, details any) {
	c.AbortWithStatusJSON(status, errorBody{Error: errorDetail{Code: code, Message: message, Details: details}})
}
