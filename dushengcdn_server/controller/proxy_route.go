package controller

import (
	"dushengcdn/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetProxyRoutes godoc
// @Summary List proxy routes
// @Tags ProxyRoutes
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/proxy-routes/ [get]
func GetProxyRoutes(c *gin.Context) {
	routes, err := service.ListProxyRoutes()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, routes)
}

// GetProxyRoute godoc
// @Summary Get proxy route detail
// @Tags ProxyRoutes
// @Produce json
// @Security BearerAuth
// @Param id path int true "Route ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/proxy-routes/{id} [get]
func GetProxyRoute(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid id")
	if !ok {
		return
	}
	route, err := service.GetProxyRoute(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, route)
}

// CreateProxyRoute godoc
// @Summary Create proxy route
// @Tags ProxyRoutes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body service.ProxyRouteInput true "Proxy route payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/proxy-routes/ [post]
func CreateProxyRoute(c *gin.Context) {
	var input service.ProxyRouteInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "invalid payload")
		return
	}
	route, err := service.CreateProxyRoute(input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, route)
}

// UpdateProxyRoute godoc
// @Summary Update proxy route
// @Tags ProxyRoutes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Route ID"
// @Param payload body service.ProxyRouteInput true "Proxy route payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/proxy-routes/{id}/update [post]
func UpdateProxyRoute(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid id")
	if !ok {
		return
	}
	var input service.ProxyRouteInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "invalid payload")
		return
	}
	route, err := service.UpdateProxyRoute(id, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, route)
}

func SwitchProxyRouteToAuthoritativeDNS(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid id")
	if !ok {
		return
	}
	// Empty body keeps the defaults; a malformed body must not be ignored.
	var input service.AuthoritativeDNSMigrationInput
	if err := decodeOptionalJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "invalid payload")
		return
	}
	route, err := service.SwitchProxyRouteToAuthoritativeDNS(id, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, route)
}

// DeleteProxyRoute godoc
// @Summary Delete proxy route
// @Tags ProxyRoutes
// @Produce json
// @Security BearerAuth
// @Param id path int true "Route ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/proxy-routes/{id}/delete [post]
func DeleteProxyRoute(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid id")
	if !ok {
		return
	}
	if err := service.DeleteProxyRoute(id); err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccessMessage(c, "")
}

func PurgeProxyRouteCache(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid id")
	if !ok {
		return
	}
	// Empty body keeps the full-purge default; a malformed body must be
	// rejected instead of silently triggering a full cache purge.
	input := service.CacheOperationInput{Scope: "all"}
	if err := decodeOptionalJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "invalid payload")
		return
	}
	result, err := service.RequestProxyRouteCachePurge(id, input)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": result})
		return
	}
	respondSuccess(c, result)
}

func WarmProxyRouteCache(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid id")
	if !ok {
		return
	}
	var input service.CacheOperationInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "invalid payload")
		return
	}
	result, err := service.RequestProxyRouteCacheWarm(id, input)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": result})
		return
	}
	respondSuccess(c, result)
}
