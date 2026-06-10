package geoip

import (
	"dushengcdn/utils/security"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxIPAPIResponseBytes int64 = 1024 * 1024

// IPAPIService 使用 ip-api.com 服务实现 GeoIPService 接口。
type IPAPIService struct {
	Client *http.Client
}

// ipAPIResponse 定义了 ip-api.com 服务返回的 JSON 响应的结构。
type ipAPIResponse struct {
	Status      string  `json:"status"`
	Message     string  `json:"message"` // 当 status 为 fail 时出现
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Region      string  `json:"region"`
	RegionName  string  `json:"regionName"`
	City        string  `json:"city"`
	Zip         string  `json:"zip"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`
	Org         string  `json:"org"`
	As          string  `json:"as"`
	Query       string  `json:"query"`
}

func (s *IPAPIService) Name() string {
	return "ip-api.com"
}

// NewIPAPIService 创建并返回一个 IPAPIService 的新实例。
func NewIPAPIService() (*IPAPIService, error) {
	return &IPAPIService{
		Client: security.NewPublicHTTPClient(5*time.Second, true),
	}, nil
}

// GetGeoInfo 使用 ip-api.com 服务检索给定 IP 地址的地理位置信息。
func (s *IPAPIService) GetGeoInfo(ip net.IP) (*GeoInfo, error) {
	// API URL, 使用 fields 参数来仅请求需要的字段
	apiURL := fmt.Sprintf("https://ip-api.com/json/%s?fields=status,message,country,countryCode,isp,org,as", url.PathEscape(ip.String()))

	client := s.Client
	if client == nil {
		client = security.NewPublicHTTPClient(5*time.Second, true)
	}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create ip-api.com request: %w", err)
	}
	req.Header.Set("User-Agent", "DuShengCDN-Server")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get geo info from ip-api.com: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ip-api.com returned non-200 status: %s", resp.Status)
	}

	var apiResp ipAPIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxIPAPIResponseBytes)).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode ip-api.com response: %w", err)
	}

	if apiResp.Status != "success" {
		return nil, fmt.Errorf("ip-api.com returned an error: %s", apiResp.Message)
	}

	return &GeoInfo{
		ISOCode:   apiResp.CountryCode,
		Name:      apiResp.Country,
		Operator:  normalizeGeoOperator(apiResp.ISP, apiResp.Org, apiResp.As),
		Latitude:  float64Pointer(apiResp.Lat),
		Longitude: float64Pointer(apiResp.Lon),
	}, nil
}

func normalizeGeoOperator(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// UpdateDatabase 对于 ip-api.com 是一个空操作，因为它是一个 Web 服务。
func (s *IPAPIService) UpdateDatabase() error {
	// 无需执行任何操作，因为数据由外部服务提供
	return nil
}

// Close 对于 ip-api.com 是一个空操作，因为没有需要关闭的持久连接。
func (s *IPAPIService) Close() error {
	// 无需执行任何操作
	return nil
}
