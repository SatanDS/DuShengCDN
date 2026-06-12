package controller

import (
	"dushengcdn/service"

	"github.com/gin-gonic/gin"
)

func GetOrigins(c *gin.Context) {
	origins, err := service.ListOrigins()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, origins)
}

func GetOrigin(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "无效的参数")
	if !ok {
		return
	}
	origin, err := service.GetOriginDetail(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, origin)
}

func CreateOrigin(c *gin.Context) {
	var input service.OriginInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	origin, err := service.CreateOrigin(input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, origin)
}

func UpdateOrigin(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "无效的参数")
	if !ok {
		return
	}
	var input service.OriginInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	origin, err := service.UpdateOrigin(id, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, origin)
}

func DeleteOrigin(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "无效的参数")
	if !ok {
		return
	}
	if err := service.DeleteOrigin(id); err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccessMessage(c, "")
}
