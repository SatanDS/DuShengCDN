package controller

import (
	"dushengcdn/common"
	"dushengcdn/service"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
)

const tlsCertificateMultipartMaxBytes int64 = 2 * 1024 * 1024

// GetTLSCertificates godoc
// @Summary List TLS certificates
// @Tags TLSCertificates
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/tls-certificates/ [get]
func GetTLSCertificates(c *gin.Context) {
	certificates, err := service.ListTLSCertificates()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, certificates)
}

// GetTLSCertificate godoc
// @Summary Get TLS certificate detail
// @Tags TLSCertificates
// @Produce json
// @Security BearerAuth
// @Param id path int true "Certificate ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/{id} [get]
func GetTLSCertificate(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid request")
	if !ok {
		return
	}

	certificate, err := service.GetTLSCertificate(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, certificate)
}

// GetTLSCertificateContent godoc
// @Summary Get TLS certificate PEM content
// @Tags TLSCertificates
// @Produce json
// @Security BearerAuth
// @Param id path int true "Certificate ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/{id}/content [post]
func GetTLSCertificateContent(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid request")
	if !ok {
		return
	}

	revealKey := c.Query("reveal_key") == "true"
	if revealKey && c.GetInt("role") < common.RoleRootUser {
		respondFailure(c, "root permission is required to reveal TLS private key")
		return
	}
	content, err := service.GetTLSCertificateContent(id, revealKey)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, content)
}

// CreateTLSCertificate godoc
// @Summary Create TLS certificate from PEM
// @Tags TLSCertificates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body service.TLSCertificateInput true "TLS certificate payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/ [post]
func CreateTLSCertificate(c *gin.Context) {
	var input service.TLSCertificateInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	certificate, err := service.CreateTLSCertificate(input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, certificate)
}

// UpdateTLSCertificate godoc
// @Summary Update TLS certificate from PEM
// @Tags TLSCertificates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Certificate ID"
// @Param payload body service.TLSCertificateInput true "TLS certificate payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/{id}/update [post]
func UpdateTLSCertificate(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid request")
	if !ok {
		return
	}

	var input service.TLSCertificateInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "invalid request")
		return
	}

	certificate, err := service.UpdateTLSCertificate(id, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, certificate)
}

// ImportTLSCertificateFile godoc
// @Summary Import TLS certificate from files
// @Tags TLSCertificates
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param name formData string true "Certificate name"
// @Param remark formData string false "Remark"
// @Param cert_file formData file true "Certificate file"
// @Param key_file formData file true "Private key file"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/import-file [post]
func ImportTLSCertificateFile(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, tlsCertificateMultipartMaxBytes)
	if err := c.Request.ParseMultipartForm(tlsCertificateMultipartMaxBytes); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"success": false,
				"message": "upload body is too large",
			})
			return
		}
		respondBadRequest(c, "invalid multipart upload")
		return
	}
	name := c.PostForm("name")
	remark := c.PostForm("remark")
	certFile, err := c.FormFile("cert_file")
	if err != nil {
		respondBadRequest(c, "缺少证书文件")
		return
	}
	keyFile, err := c.FormFile("key_file")
	if err != nil {
		respondBadRequest(c, "缺少私钥文件")
		return
	}
	certificate, err := service.CreateTLSCertificateFromFiles(name, certFile, keyFile, remark)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, certificate)
}

// DeleteTLSCertificate godoc
// @Summary Delete TLS certificate
// @Tags TLSCertificates
// @Produce json
// @Security BearerAuth
// @Param id path int true "Certificate ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/{id}/delete [post]
func DeleteTLSCertificate(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "无效的参数")
	if !ok {
		return
	}
	if err := service.DeleteTLSCertificate(id); err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccessMessage(c, "")
}

// ApplyTLSCertificate godoc
// @Summary Apply TLS certificate via ACME
// @Tags TLSCertificates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body service.TLSApplyInput true "TLS apply payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/apply [post]
func ApplyTLSCertificate(c *gin.Context) {
	var input service.TLSApplyInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	certificate, err := service.ApplyTLSCertificate(input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, certificate)
}

// UpdateAcmeCertificate godoc
// @Summary Update ACME TLS certificate
// @Tags TLSCertificates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Certificate ID"
// @Param payload body service.TLSApplyInput true "TLS apply payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/{id}/update-acme [post]
func UpdateAcmeCertificate(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid request")
	if !ok {
		return
	}

	var input service.TLSApplyInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	certificate, err := service.UpdateAcmeCertificate(id, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, certificate)
}

// ConvertTLSCertificateToAcme godoc
// @Summary Convert uploaded TLS certificate to ACME managed certificate
// @Tags TLSCertificates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Certificate ID"
// @Param payload body service.TLSApplyInput true "TLS apply payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/{id}/convert-acme [post]
func ConvertTLSCertificateToAcme(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "invalid request")
	if !ok {
		return
	}

	var input service.TLSApplyInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	certificate, err := service.ConvertTLSCertificateToAcme(id, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, certificate)
}

// RenewTLSCertificate godoc
// @Summary Renew TLS certificate
// @Tags TLSCertificates
// @Produce json
// @Security BearerAuth
// @Param id path int true "Certificate ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/tls-certificates/{id}/renew [post]
func RenewTLSCertificate(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "无效的参数")
	if !ok {
		return
	}
	certificate, err := service.RenewTLSCertificate(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, certificate)
}
