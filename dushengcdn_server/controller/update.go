package controller

import (
	"context"
	"dushengcdn/service"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

type confirmManualUpgradeRequest struct {
	UploadToken string `json:"upload_token"`
}

type serverUpgradeRequest struct {
	Channel string `json:"channel"`
}

// GetLatestRelease godoc
// @Summary Get latest GitHub release
// @Tags Update
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/update/latest-release [get]
func GetLatestRelease(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	release, err := service.GetLatestServerRelease(ctx, c.Query("channel"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    release,
	})
}

// UpgradeServer godoc
// @Summary Upgrade server binary from latest GitHub release
// @Tags Update
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/update/upgrade [post]
func UpgradeServer(c *gin.Context) {
	var request serverUpgradeRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "无效的参数",
			})
			return
		}
	}
	release, err := service.ScheduleServerUpgrade(request.Channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "服务升级任务已启动，下载完成后将自动重启。",
		"data":    release,
	})
}

// StreamServerUpgradeLogs godoc
// @Summary Stream server upgrade logs over websocket
// @Tags Update
// @Router /api/update/logs/ws [get]
func StreamServerUpgradeLogs(c *gin.Context) {
	if rejectInvalidWebSocketOrigin(c) {
		return
	}
	websocket.Handler(func(conn *websocket.Conn) {
		defer func() {
			_ = conn.Close()
		}()

		updates, unsubscribe := service.SubscribeServerUpgradeStream()
		defer unsubscribe()

		heartbeatTicker := time.NewTicker(15 * time.Second)
		defer heartbeatTicker.Stop()

		for {
			select {
			case snapshot, ok := <-updates:
				if !ok {
					return
				}
				if err := websocket.JSON.Send(conn, snapshot); err != nil {
					return
				}
			case <-heartbeatTicker.C:
				if err := websocket.JSON.Send(conn, service.ServerUpgradeStreamSnapshot{}); err != nil {
					return
				}
			case <-c.Request.Context().Done():
				return
			}
		}
	}).ServeHTTP(c.Writer, c.Request)
}

// UploadManualServerBinary godoc
// @Summary Upload server binary and inspect version before upgrade
// @Tags Update
// @Accept mpfd
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/update/manual-upload [post]
func UploadManualServerBinary(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.ManualServerBinaryMaxBytes())
	fileHeader, err := c.FormFile("binary")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": manualUploadFormErrorMessage(err),
		})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "读取上传文件失败。",
		})
		return
	}
	defer func() {
		_ = file.Close()
	}()
	checksumFile, checksumClose, err := openManualUploadFormFile(c, "checksum", "Please upload the matching .sha256 file.")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	defer checksumClose()
	signatureFile, signatureClose, err := openManualUploadFormFile(c, "signature", "Please upload the matching .sig file.")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	defer signatureClose()

	info, err := service.UploadManualServerBinary(c.Request.Context(), fileHeader.Filename, file, checksumFile, signatureFile)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	message := strings.TrimSpace(info.ComparisonMessage)
	if message == "" {
		message = "已完成上传并检查升级包版本。"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"data":    info,
	})
}

func manualUploadFormErrorMessage(err error) string {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "request body too large") {
		return "上传二进制超过大小限制。"
	}
	return "请先选择要上传的服务端二进制文件。"
}

func openManualUploadFormFile(c *gin.Context, field string, missingMessage string) (multipart.File, func(), error) {
	fileHeader, err := c.FormFile(field)
	if err != nil {
		return nil, func() {}, errors.New(missingMessage)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, func() {}, err
	}
	return file, func() {
		_ = file.Close()
	}, nil
}

// ConfirmManualServerUpgrade godoc
// @Summary Confirm upgrade with previously uploaded server binary
// @Tags Update
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/update/manual-upgrade [post]
func ConfirmManualServerUpgrade(c *gin.Context) {
	var request confirmManualUpgradeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "升级确认参数无效。",
		})
		return
	}

	info, err := service.ConfirmManualServerUpgrade(request.UploadToken)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "服务升级任务已启动，确认无误后将自动重启。",
		"data":    info,
	})
}
