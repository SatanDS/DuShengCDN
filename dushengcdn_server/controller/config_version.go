package controller

import (
	"dushengcdn/service"

	"github.com/gin-gonic/gin"
)

// GetConfigVersions godoc
// @Summary List config versions
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/config-versions/ [get]
func GetConfigVersions(c *gin.Context) {
	versions, err := service.ListConfigVersions()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, versions)
}

// GetConfigVersion godoc
// @Summary Get config version detail
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param id path int true "Version ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/config-versions/{id} [get]
func GetConfigVersion(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid id")
	if !ok {
		return
	}
	version, err := service.GetConfigVersionDetail(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, version)
}

// GetActiveConfigVersion godoc
// @Summary Get active config version
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/config-versions/active [get]
func GetActiveConfigVersion(c *gin.Context) {
	version, err := service.GetActiveConfigVersion()
	if err != nil {
		respondFailure(c, "当前没有激活版本")
		return
	}
	respondSuccess(c, version)
}

// PreviewConfigVersion godoc
// @Summary Preview config rendering
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/config-versions/preview [get]
func PreviewConfigVersion(c *gin.Context) {
	preview, err := service.PreviewConfigVersion()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, preview)
}

// DiffConfigVersion godoc
// @Summary Diff current draft against active version
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/config-versions/diff [get]
func DiffConfigVersion(c *gin.Context) {
	diff, err := service.DiffConfigVersion()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, diff)
}

// PublishConfigVersion godoc
// @Summary Publish a new config version
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/config-versions/publish [post]
func PublishConfigVersion(c *gin.Context) {
	username := c.GetString("username")
	force := c.Query("force") == "true"
	result, err := service.PublishConfigVersion(username, force)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, service.BuildConfigVersionDetailForAdmin(result.Version))
}

// ActivateConfigVersion godoc
// @Summary Activate an existing config version
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param id path int true "Version ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/config-versions/{id}/activate [post]
func ActivateConfigVersion(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid id")
	if !ok {
		return
	}
	version, err := service.ActivateConfigVersion(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, service.BuildConfigVersionDetailForAdmin(version))
}

type CleanupConfigVersionRequest struct {
	KeepCount int `json:"keep_count" binding:"required,min=3"`
}

// CleanupConfigVersions godoc
// @Summary Cleanup old config versions
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param request body CleanupConfigVersionRequest true "Cleanup request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/config-versions/cleanup [post]
func CleanupConfigVersions(c *gin.Context) {
	var req CleanupConfigVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondFailure(c, "参数错误")
		return
	}

	deletedCount, err := service.CleanupConfigVersions(req.KeepCount)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}

	respondSuccessWithExtras(c, map[string]interface{}{"deleted_count": deletedCount}, gin.H{"message": "清理成功"})
}
