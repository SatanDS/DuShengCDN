package controller

import (
	"dushengcdn/model"
	"dushengcdn/service"
	"strings"

	"github.com/gin-gonic/gin"
)

type DnsAccountInput struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Authorization string `json:"authorization"`
}

// GetDnsAccounts godoc
// @Summary List DNS accounts
// @Tags DnsAccounts
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/dns-accounts/ [get]
func GetDnsAccounts(c *gin.Context) {
	accounts, err := model.ListDnsAccounts()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, accounts)
}

// CreateDnsAccount godoc
// @Summary Create DNS account
// @Tags DnsAccounts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body DnsAccountInput true "DNS account payload"
// @Success 200 {object} map[string]interface{}
// @Router /api/dns-accounts/ [post]
func CreateDnsAccount(c *gin.Context) {
	var input DnsAccountInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}

	account := &model.DnsAccount{
		Name:          input.Name,
		Type:          input.Type,
		Authorization: input.Authorization,
	}
	if err := service.NormalizeDNSAccountAuthorization(account); err != nil {
		respondFailure(c, "DNS 账号凭据格式无效："+err.Error())
		return
	}

	if err := account.Insert(); err != nil {
		respondFailure(c, err.Error())
		return
	}

	respondSuccess(c, account)
}

// UpdateDnsAccount godoc
// @Summary Update DNS account
// @Tags DnsAccounts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "DNS Account ID"
// @Param payload body DnsAccountInput true "DNS account payload"
// @Success 200 {object} map[string]interface{}
// @Router /api/dns-accounts/{id}/update [post]
func UpdateDnsAccount(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "无效的参数")
	if !ok {
		return
	}

	var input DnsAccountInput
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}

	account, err := model.GetDnsAccountByID(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}

	account.Name = input.Name
	account.Type = input.Type
	if strings.TrimSpace(input.Authorization) != "" {
		account.Authorization = input.Authorization
	}
	if err := service.NormalizeDNSAccountAuthorization(account); err != nil {
		respondFailure(c, "DNS 账号凭据格式无效："+err.Error())
		return
	}

	if err := account.Update(); err != nil {
		respondFailure(c, err.Error())
		return
	}

	respondSuccess(c, account)
}

// DeleteDnsAccount godoc
// @Summary Delete DNS account
// @Tags DnsAccounts
// @Produce json
// @Security BearerAuth
// @Param id path int true "DNS Account ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/dns-accounts/{id}/delete [post]
func DeleteDnsAccount(c *gin.Context) {
	id, ok := parseUintParamWithMessage(c, "id", "无效的参数")
	if !ok {
		return
	}

	account, err := model.GetDnsAccountByID(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}

	// Verify no cert uses this before deleting
	var count int64
	model.DB.Model(&model.TLSCertificate{}).Where("dns_account_id = ?", id).Count(&count)
	if count > 0 {
		respondFailure(c, "该 DNS 账号已被证书使用，无法删除")
		return
	}

	if err := account.Delete(); err != nil {
		respondFailure(c, err.Error())
		return
	}

	respondSuccessMessage(c, "")
}
