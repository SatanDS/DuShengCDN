package controller

import (
	"dushengcdn/utils/security"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const invalidParamsMessage = "无效的参数"

func respondSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

func respondSuccessWithExtras(c *gin.Context, data any, extras gin.H) {
	payload := gin.H{
		"success": true,
		"message": "",
		"data":    data,
	}
	for key, value := range extras {
		payload[key] = value
	}
	c.JSON(http.StatusOK, payload)
}

func respondSuccessMessage(c *gin.Context, message string) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": sanitizeResponseMessage(message),
	})
}

func respondFailure(c *gin.Context, message string) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": sanitizeResponseMessage(message),
	})
}

func respondBadRequest(c *gin.Context, message string) {
	if message == "" {
		message = invalidParamsMessage
	}
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"message": sanitizeResponseMessage(message),
	})
}

func respondUnauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"message": sanitizeResponseMessage(message),
	})
}

func parseUintParam(c *gin.Context, key string) (uint, bool) {
	return parseUintParamWithMessage(c, key, "invalid parameter")
}

func parseUintParamWithMessage(c *gin.Context, key string, message string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(key), 10, 64)
	if err != nil || id == 0 {
		respondBadRequest(c, message)
		return 0, false
	}
	return uint(id), true
}

func sanitizeResponseMessage(message string) string {
	return security.RedactSensitiveText(message)
}

func decodeJSONBody(body io.Reader, target any) error {
	return json.NewDecoder(body).Decode(target)
}

func decodeOptionalJSONBody(body io.Reader, target any) error {
	if err := json.NewDecoder(body).Decode(target); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
