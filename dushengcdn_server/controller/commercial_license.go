package controller

import (
	"dushengcdn/service"

	"github.com/gin-gonic/gin"
)

// GetCommercialLicense godoc
// @Summary Get commercial license status
// @Tags License
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/license/status [get]
func GetCommercialLicense(c *gin.Context) {
	view, err := service.GetCommercialLicenseStatus()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, view)
}

// InstallCommercialLicense godoc
// @Summary Install commercial license
// @Tags License
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body service.CommercialLicenseInstallInput true "License payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/license/install [post]
func InstallCommercialLicense(c *gin.Context) {
	var input service.CommercialLicenseInstallInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "")
		return
	}
	view, err := service.InstallCommercialLicense(input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, view)
}

// ClearCommercialLicense godoc
// @Summary Clear commercial license
// @Tags License
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/license/clear [post]
func ClearCommercialLicense(c *gin.Context) {
	view, err := service.ClearCommercialLicense()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, view)
}

// GetCommercialLicenseIssuer godoc
// @Summary Get commercial license issuer status
// @Tags License
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/license/issuer [get]
func GetCommercialLicenseIssuer(c *gin.Context) {
	respondSuccess(c, service.GetCommercialLicenseIssuerStatus())
}

// IssueCommercialLicense godoc
// @Summary Issue commercial license token
// @Tags License
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body service.CommercialLicenseIssueInput true "License issue payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/license/issue [post]
func IssueCommercialLicense(c *gin.Context) {
	var input service.CommercialLicenseIssueInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "")
		return
	}
	result, err := service.IssueCommercialLicense(input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, result)
}
