package controller

import (
	"dushengcdn/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListConfigReleasePlans godoc
// @Summary List config release plans
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/config-release-plans/ [get]
func ListConfigReleasePlans(c *gin.Context) {
	plans, err := service.ListConfigReleasePlans()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, plans)
}

// GetConfigReleasePlan godoc
// @Summary Get config release plan
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param id path int true "Plan ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/config-release-plans/{id} [get]
func GetConfigReleasePlan(c *gin.Context) {
	id, ok := parseConfigReleasePlanID(c)
	if !ok {
		return
	}
	plan, err := service.GetConfigReleasePlan(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, plan)
}

// CreateConfigReleasePlan godoc
// @Summary Create config release plan
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param payload body service.ConfigReleasePlanInput true "Release plan payload"
// @Success 200 {object} map[string]interface{}
// @Router /api/config-release-plans/ [post]
func CreateConfigReleasePlan(c *gin.Context) {
	var input service.ConfigReleasePlanInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondBadRequest(c, "invalid request payload")
		return
	}
	plan, err := service.CreateConfigReleasePlan(c.GetString("username"), input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, plan)
}

// StartConfigReleasePlan godoc
// @Summary Start config release plan
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param id path int true "Plan ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/config-release-plans/{id}/start [post]
func StartConfigReleasePlan(c *gin.Context) {
	id, ok := parseConfigReleasePlanID(c)
	if !ok {
		return
	}
	plan, err := service.StartConfigReleasePlan(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, plan)
}

// EvaluateConfigReleasePlan godoc
// @Summary Evaluate config release plan
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param id path int true "Plan ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/config-release-plans/{id}/evaluate [post]
func EvaluateConfigReleasePlan(c *gin.Context) {
	id, ok := parseConfigReleasePlanID(c)
	if !ok {
		return
	}
	result, err := service.EvaluateConfigReleasePlan(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, result)
}

// AdvanceConfigReleasePlan godoc
// @Summary Advance config release plan
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param id path int true "Plan ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/config-release-plans/{id}/advance [post]
func AdvanceConfigReleasePlan(c *gin.Context) {
	id, ok := parseConfigReleasePlanID(c)
	if !ok {
		return
	}
	plan, err := service.AdvanceConfigReleasePlan(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, plan)
}

// CompleteConfigReleasePlan godoc
// @Summary Complete config release plan
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param id path int true "Plan ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/config-release-plans/{id}/complete [post]
func CompleteConfigReleasePlan(c *gin.Context) {
	id, ok := parseConfigReleasePlanID(c)
	if !ok {
		return
	}
	plan, err := service.CompleteConfigReleasePlan(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, plan)
}

// FailConfigReleasePlan godoc
// @Summary Fail config release plan
// @Tags ConfigVersions
// @Produce json
// @Security BearerAuth
// @Param id path int true "Plan ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/config-release-plans/{id}/fail [post]
func FailConfigReleasePlan(c *gin.Context) {
	id, ok := parseConfigReleasePlanID(c)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := service.FailConfigReleasePlan(id, req.Reason); err != nil {
		respondFailure(c, err.Error())
		return
	}
	plan, err := service.GetConfigReleasePlan(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, plan)
}

func parseConfigReleasePlanID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respondBadRequest(c, "invalid id")
		return 0, false
	}
	return uint(id), true
}
