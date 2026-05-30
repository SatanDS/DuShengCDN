package service

import (
	"bytes"
	"context"
	"dushengcdn/model"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	cloudflareAPIBaseURL           = "https://api.cloudflare.com/client/v4"
	cloudflareDefaultRecordTTL     = 1
	cloudflareDNSProviderType      = "cloudflare"
	DNSRecordSyncStatusSuccess     = "success"
	DNSRecordSyncStatusFailed      = "failed"
	DDOSProtectionModeOff          = "off"
	DDOSProtectionModeManual       = "manual"
	DDOSProtectionModeAuto         = "auto"
	defaultCloudflareHTTPTimeout   = 15 * time.Second
	defaultCloudflareSyncUserAgent = "DuShengCDN/CloudflareDNS"
)

type CloudflareCredentials struct {
	APIToken string `json:"api_token"`
}

type cloudflareCredentialsAlias struct {
	APIToken      string `json:"api_token"`
	APITokenCamel string `json:"apiToken"`
	Token         string `json:"token"`
}

type CloudflareZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CloudflareDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

type CloudflareDNSUpsertInput struct {
	ZoneID  string
	Type    string
	Name    string
	Content string
	Proxied bool
	TTL     int
}

type CloudflareDNSSyncInput struct {
	ZoneID           string
	Type             string
	Name             string
	Contents         []string
	Proxied          bool
	TTL              int
	ManagedRecordIDs map[string]string
}

type cloudflareClient struct {
	apiToken   string
	baseURL    string
	httpClient *http.Client
}

type cloudflareAPIResponse[T any] struct {
	Success  bool                   `json:"success"`
	Errors   []cloudflareAPIError   `json:"errors"`
	Messages []cloudflareAPIMessage `json:"messages"`
	Result   T                      `json:"result"`
}

type cloudflareAPIListResponse[T any] struct {
	Success    bool                   `json:"success"`
	Errors     []cloudflareAPIError   `json:"errors"`
	Messages   []cloudflareAPIMessage `json:"messages"`
	Result     []T                    `json:"result"`
	ResultInfo struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalPages int `json:"total_pages"`
		Count      int `json:"count"`
		TotalCount int `json:"total_count"`
	} `json:"result_info"`
}

type cloudflareAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cloudflareAPIMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func parseCloudflareCredentials(account *model.DnsAccount) (*CloudflareCredentials, error) {
	if account == nil {
		return nil, errors.New("DNS 账号不存在")
	}
	if strings.ToLower(strings.TrimSpace(account.Type)) != cloudflareDNSProviderType {
		return nil, fmt.Errorf("DNS 账号类型 %s 不支持自动 DNS，同步功能目前仅支持 Cloudflare", account.Type)
	}
	var credentials CloudflareCredentials
	if err := json.Unmarshal([]byte(account.Authorization), &credentials); err != nil {
		return nil, fmt.Errorf("Cloudflare 凭据格式无效：%w", err)
	}
	credentials.APIToken = strings.TrimSpace(credentials.APIToken)
	if credentials.APIToken == "" {
		return nil, errors.New("Cloudflare DNS 账号缺少 api_token")
	}
	return &credentials, nil
}

func parseCloudflareCredentialsV2(account *model.DnsAccount) (*CloudflareCredentials, error) {
	if account == nil {
		return nil, errors.New("DNS 账号不存在")
	}
	if strings.ToLower(strings.TrimSpace(account.Type)) != cloudflareDNSProviderType {
		return nil, fmt.Errorf("DNS 账号类型 %s 不支持自动 DNS，同步功能目前仅支持 Cloudflare", account.Type)
	}
	token := parseCloudflareAPIToken(account.Authorization)
	if token == "" {
		return nil, errors.New("Cloudflare DNS 账号缺少 api_token")
	}
	return &CloudflareCredentials{APIToken: token}, nil
}

func NormalizeDNSAccountAuthorization(account *model.DnsAccount) error {
	if account == nil {
		return errors.New("DNS 账号不存在")
	}
	account.Type = strings.ToLower(strings.TrimSpace(account.Type))
	switch account.Type {
	case cloudflareDNSProviderType:
		token := parseCloudflareAPIToken(account.Authorization)
		if token == "" {
			return errors.New("Cloudflare DNS 账号缺少 api_token")
		}
		raw, err := json.Marshal(CloudflareCredentials{APIToken: token})
		if err != nil {
			return err
		}
		account.Authorization = string(raw)
		return nil
	case "":
		return errors.New("DNS 账号类型不能为空")
	default:
		return fmt.Errorf("DNS 账号类型 %s 暂不支持", account.Type)
	}
}

func parseCloudflareAPIToken(raw string) string {
	text := normalizeCloudflareAPIToken(raw)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "{") {
		var credentials cloudflareCredentialsAlias
		if err := json.Unmarshal([]byte(text), &credentials); err != nil {
			return ""
		}
		return normalizeCloudflareAPIToken(firstNonEmptyCloudflareCredential(credentials.APIToken, credentials.APITokenCamel, credentials.Token))
	}
	var quoted string
	if err := json.Unmarshal([]byte(text), &quoted); err == nil {
		return normalizeCloudflareAPIToken(quoted)
	}
	return text
}

func normalizeCloudflareAPIToken(raw string) string {
	token := strings.TrimSpace(raw)
	token = strings.Trim(token, "\"'")
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	token = strings.ReplaceAll(token, "\r", "")
	token = strings.ReplaceAll(token, "\n", "")
	return strings.TrimSpace(token)
}

func firstNonEmptyCloudflareCredential(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func newCloudflareClientFromAccount(account *model.DnsAccount) (*cloudflareClient, error) {
	credentials, err := parseCloudflareCredentialsV2(account)
	if err != nil {
		return nil, err
	}
	return &cloudflareClient{
		apiToken: credentials.APIToken,
		baseURL:  cloudflareAPIBaseURL,
		httpClient: &http.Client{
			Timeout: defaultCloudflareHTTPTimeout,
		},
	}, nil
}

func (client *cloudflareClient) do(ctx context.Context, method string, path string, query url.Values, body any, out any) error {
	if client == nil {
		return errors.New("Cloudflare client is nil")
	}
	baseURL := strings.TrimRight(client.baseURL, "/") + path
	if len(query) > 0 {
		baseURL += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+client.apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultCloudflareSyncUserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 Cloudflare API 失败：%w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return fmt.Errorf("读取 Cloudflare API 响应失败：%w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Cloudflare API 返回异常状态 %s：%s", resp.Status, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("解析 Cloudflare API 响应失败：%w", err)
	}
	return nil
}

func cloudflareErrorMessage(errorsList []cloudflareAPIError) string {
	if len(errorsList) == 0 {
		return "Cloudflare API 返回失败"
	}
	parts := make([]string, 0, len(errorsList))
	for _, item := range errorsList {
		message := strings.TrimSpace(item.Message)
		if message == "" {
			message = fmt.Sprintf("错误码 %d", item.Code)
		}
		parts = append(parts, message)
	}
	return strings.Join(parts, "；")
}

func (client *cloudflareClient) VerifyToken(ctx context.Context) error {
	var response cloudflareAPIResponse[map[string]any]
	if err := client.do(ctx, http.MethodGet, "/user/tokens/verify", nil, nil, &response); err != nil {
		return err
	}
	if !response.Success {
		return errors.New(cloudflareErrorMessage(response.Errors))
	}
	return nil
}

func (client *cloudflareClient) ListZones(ctx context.Context, name string) ([]CloudflareZone, error) {
	query := url.Values{}
	if strings.TrimSpace(name) != "" {
		query.Set("name", strings.TrimSpace(name))
	}
	query.Set("per_page", "50")
	var response cloudflareAPIListResponse[CloudflareZone]
	if err := client.do(ctx, http.MethodGet, "/zones", query, nil, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, errors.New(cloudflareErrorMessage(response.Errors))
	}
	return response.Result, nil
}

func (client *cloudflareClient) FindBestZoneForDomain(ctx context.Context, domain string) (*CloudflareZone, error) {
	normalizedDomain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if normalizedDomain == "" {
		return nil, errors.New("域名不能为空")
	}
	labels := strings.Split(normalizedDomain, ".")
	for index := 0; index < len(labels)-1; index++ {
		candidate := strings.Join(labels[index:], ".")
		zones, err := client.ListZones(ctx, candidate)
		if err != nil {
			return nil, err
		}
		for _, zone := range zones {
			if strings.EqualFold(zone.Name, candidate) {
				return &zone, nil
			}
		}
	}
	return nil, fmt.Errorf("Cloudflare 中没有找到域名 %s 对应的 Zone，请确认该域名已接入 Cloudflare", domain)
}

func (client *cloudflareClient) ListDNSRecords(ctx context.Context, zoneID string, recordType string, name string) ([]CloudflareDNSRecord, error) {
	zoneID = strings.TrimSpace(zoneID)
	if zoneID == "" {
		return nil, errors.New("Cloudflare Zone ID 不能为空")
	}
	query := url.Values{}
	query.Set("per_page", "100")
	if strings.TrimSpace(recordType) != "" {
		query.Set("type", strings.ToUpper(strings.TrimSpace(recordType)))
	}
	if strings.TrimSpace(name) != "" {
		query.Set("name", strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), "."))
	}
	var response cloudflareAPIListResponse[CloudflareDNSRecord]
	if err := client.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(zoneID)+"/dns_records", query, nil, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, errors.New(cloudflareErrorMessage(response.Errors))
	}
	return response.Result, nil
}

func (client *cloudflareClient) UpsertDNSRecord(ctx context.Context, input CloudflareDNSUpsertInput) (*CloudflareDNSRecord, error) {
	input.ZoneID = strings.TrimSpace(input.ZoneID)
	input.Type = normalizeDNSRecordType(input.Type)
	input.Name = normalizeDNSRecordName(input.Name)
	input.Content = strings.TrimSpace(input.Content)
	if input.TTL <= 0 {
		input.TTL = cloudflareDefaultRecordTTL
	}
	if input.ZoneID == "" || input.Name == "" || input.Content == "" {
		return nil, errors.New("Cloudflare DNS 记录参数不完整")
	}
	if err := validateDNSRecordContent(input.Type, input.Content); err != nil {
		return nil, err
	}

	records, err := client.ListDNSRecords(ctx, input.ZoneID, input.Type, input.Name)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"type":    input.Type,
		"name":    input.Name,
		"content": input.Content,
		"ttl":     input.TTL,
		"proxied": input.Proxied,
	}
	if len(records) == 0 {
		var response cloudflareAPIResponse[CloudflareDNSRecord]
		if err := client.do(ctx, http.MethodPost, "/zones/"+url.PathEscape(input.ZoneID)+"/dns_records", nil, payload, &response); err != nil {
			return nil, err
		}
		if !response.Success {
			return nil, errors.New(cloudflareErrorMessage(response.Errors))
		}
		return &response.Result, nil
	}

	record := records[0]
	var response cloudflareAPIResponse[CloudflareDNSRecord]
	if err := client.do(ctx, http.MethodPut, "/zones/"+url.PathEscape(input.ZoneID)+"/dns_records/"+url.PathEscape(record.ID), nil, payload, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, errors.New(cloudflareErrorMessage(response.Errors))
	}
	return &response.Result, nil
}

func (client *cloudflareClient) SyncDNSRecords(ctx context.Context, input CloudflareDNSSyncInput) ([]CloudflareDNSRecord, error) {
	input.ZoneID = strings.TrimSpace(input.ZoneID)
	input.Type = normalizeDNSRecordType(input.Type)
	input.Name = normalizeDNSRecordName(input.Name)
	if input.TTL <= 0 {
		input.TTL = cloudflareDefaultRecordTTL
	}
	contents, err := normalizeDNSRecordContents(input.Type, input.Contents)
	if err != nil {
		return nil, err
	}
	if input.ZoneID == "" || input.Name == "" || len(contents) == 0 {
		return nil, errors.New("Cloudflare DNS record sync parameters are incomplete")
	}
	if input.Type == "CNAME" && len(contents) > 1 {
		return nil, errors.New("CNAME record sync only supports one target")
	}

	records, err := client.ListDNSRecords(ctx, input.ZoneID, input.Type, input.Name)
	if err != nil {
		return nil, err
	}
	existingByContent := make(map[string]CloudflareDNSRecord, len(records))
	for _, record := range records {
		existingByContent[strings.TrimSpace(record.Content)] = record
	}

	desiredKeys := make(map[string]struct{}, len(contents))
	result := make([]CloudflareDNSRecord, 0, len(contents))
	for _, content := range contents {
		desiredKeys[dnsRecordStorageKey(input.Name, content)] = struct{}{}
		payload := map[string]any{
			"type":    input.Type,
			"name":    input.Name,
			"content": content,
			"ttl":     input.TTL,
			"proxied": input.Proxied,
		}
		if record, ok := existingByContent[content]; ok {
			updated, err := client.putDNSRecord(ctx, input.ZoneID, record.ID, payload)
			if err != nil {
				return nil, err
			}
			result = append(result, *updated)
			continue
		}
		created, err := client.postDNSRecord(ctx, input.ZoneID, payload)
		if err != nil {
			return nil, err
		}
		result = append(result, *created)
	}

	for key, recordID := range input.ManagedRecordIDs {
		if isLegacyDNSRecordStorageKey(key) {
			continue
		}
		if _, ok := desiredKeys[key]; ok {
			continue
		}
		if strings.TrimSpace(recordID) == "" {
			continue
		}
		if err := client.DeleteDNSRecord(ctx, input.ZoneID, recordID); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (client *cloudflareClient) postDNSRecord(ctx context.Context, zoneID string, payload map[string]any) (*CloudflareDNSRecord, error) {
	var response cloudflareAPIResponse[CloudflareDNSRecord]
	if err := client.do(ctx, http.MethodPost, "/zones/"+url.PathEscape(zoneID)+"/dns_records", nil, payload, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, errors.New(cloudflareErrorMessage(response.Errors))
	}
	return &response.Result, nil
}

func (client *cloudflareClient) putDNSRecord(ctx context.Context, zoneID string, recordID string, payload map[string]any) (*CloudflareDNSRecord, error) {
	var response cloudflareAPIResponse[CloudflareDNSRecord]
	if err := client.do(ctx, http.MethodPut, "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(recordID), nil, payload, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, errors.New(cloudflareErrorMessage(response.Errors))
	}
	return &response.Result, nil
}

func (client *cloudflareClient) DeleteDNSRecord(ctx context.Context, zoneID string, recordID string) error {
	zoneID = strings.TrimSpace(zoneID)
	recordID = strings.TrimSpace(recordID)
	if zoneID == "" || recordID == "" {
		return nil
	}
	var response cloudflareAPIResponse[map[string]string]
	if err := client.do(ctx, http.MethodDelete, "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(recordID), nil, nil, &response); err != nil {
		return err
	}
	if !response.Success {
		return errors.New(cloudflareErrorMessage(response.Errors))
	}
	return nil
}

func VerifyCloudflareDnsAccount(ctx context.Context, account *model.DnsAccount) error {
	client, err := newCloudflareClientFromAccount(account)
	if err != nil {
		return err
	}
	return client.VerifyToken(ctx)
}

func normalizeDNSRecordType(raw string) string {
	recordType := strings.ToUpper(strings.TrimSpace(raw))
	switch recordType {
	case "AAAA", "CNAME":
		return recordType
	default:
		return "A"
	}
}

func normalizeDNSRecordName(raw string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
}

func validateDNSRecordContent(recordType string, content string) error {
	recordType = normalizeDNSRecordType(recordType)
	content = strings.TrimSpace(content)
	switch recordType {
	case "A":
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() == nil {
			return errors.New("A 记录内容必须是 IPv4 地址")
		}
	case "AAAA":
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() != nil {
			return errors.New("AAAA 记录内容必须是 IPv6 地址")
		}
	case "CNAME":
		if normalizeDNSRecordName(content) == "" || net.ParseIP(content) != nil {
			return errors.New("CNAME 记录内容必须是域名")
		}
	default:
		return fmt.Errorf("不支持的 DNS 记录类型：%s", recordType)
	}
	return nil
}

func normalizeDNSRecordContents(recordType string, contents []string) ([]string, error) {
	result := make([]string, 0, len(contents))
	seen := make(map[string]struct{}, len(contents))
	for _, content := range contents {
		normalized := strings.TrimSpace(content)
		if normalized == "" {
			continue
		}
		if err := validateDNSRecordContent(recordType, normalized); err != nil {
			return nil, err
		}
		if recordType == "CNAME" {
			normalized = normalizeDNSRecordName(normalized)
		} else if ip := net.ParseIP(normalized); ip != nil {
			normalized = ip.String()
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil, errors.New("DNS record content cannot be empty")
	}
	return result, nil
}

func splitDNSRecordContent(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	parts := strings.FieldsFunc(content, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			values = append(values, item)
		}
	}
	return values
}

func dnsRecordStorageKey(recordName string, content string) string {
	return normalizeDNSRecordName(recordName) + "|" + strings.TrimSpace(content)
}

func isLegacyDNSRecordStorageKey(key string) bool {
	name, content, ok := strings.Cut(key, "|")
	return !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(content) == ""
}

func encodeDNSRecordIDs(records map[string]string) string {
	if len(records) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(records))
	for _, key := range keys {
		ordered[key] = records[key]
	}
	raw, err := json.Marshal(ordered)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func decodeDNSRecordIDs(raw string) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}
