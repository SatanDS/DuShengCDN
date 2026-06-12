package service

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"dushengcdn/common"
	"dushengcdn/model"
)

type ConfigPreflightStatus string

const (
	ConfigPreflightStatusPass    ConfigPreflightStatus = "pass"
	ConfigPreflightStatusWarning ConfigPreflightStatus = "warning"
	ConfigPreflightStatusError   ConfigPreflightStatus = "error"
	ConfigPreflightStatusSkipped ConfigPreflightStatus = "skipped"
)

type ConfigPreflightCheck struct {
	Key     string                `json:"key"`
	Title   string                `json:"title"`
	Status  ConfigPreflightStatus `json:"status"`
	Message string                `json:"message"`
	RouteID uint                  `json:"route_id,omitempty"`
	Site    string                `json:"site,omitempty"`
	Domain  string                `json:"domain,omitempty"`
	Details []string              `json:"details,omitempty"`
}

type ConfigPreflightReport struct {
	Passed       bool                   `json:"passed"`
	ErrorCount   int                    `json:"error_count"`
	WarningCount int                    `json:"warning_count"`
	Checks       []ConfigPreflightCheck `json:"checks"`
}

type configPreflightBlockingError struct {
	report *ConfigPreflightReport
}

func (err configPreflightBlockingError) Error() string {
	if err.report == nil || err.report.ErrorCount == 0 {
		return "发布前检查未通过"
	}
	messages := make([]string, 0, err.report.ErrorCount)
	for _, check := range err.report.Checks {
		if check.Status != ConfigPreflightStatusError {
			continue
		}
		message := strings.TrimSpace(check.Message)
		if message == "" {
			message = strings.TrimSpace(check.Title)
		}
		if message != "" {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 {
		return "发布前检查未通过"
	}
	return "发布前检查未通过：" + strings.Join(messages, "；")
}

func (report *ConfigPreflightReport) add(check ConfigPreflightCheck) {
	if report == nil {
		return
	}
	if check.Status == "" {
		check.Status = ConfigPreflightStatusPass
	}
	switch check.Status {
	case ConfigPreflightStatusError:
		report.ErrorCount++
	case ConfigPreflightStatusWarning:
		report.WarningCount++
	}
	report.Checks = append(report.Checks, check)
	report.Passed = report.ErrorCount == 0
}

func (report *ConfigPreflightReport) blockingError() error {
	if report == nil || report.ErrorCount == 0 {
		return nil
	}
	return configPreflightBlockingError{report: report}
}

func buildConfigPublishPreflightReport(routes []*model.ProxyRoute, cfg openRestyConfigSnapshot, context proxyRouteTLSCertificateLoader) *ConfigPreflightReport {
	report := &ConfigPreflightReport{Passed: true, Checks: []ConfigPreflightCheck{}}
	checkConfigPreflightDomains(report, routes)
	checkConfigPreflightCertificates(report, routes, context)
	checkConfigPreflightOrigins(report, routes, context)
	checkConfigPreflightCachePath(report, cfg)
	checkConfigPreflightTemplate(report)
	checkConfigPreflightNodePools(report, routes)
	checkConfigPreflightActiveProbes(report)
	report.Passed = report.ErrorCount == 0
	return report
}

func checkConfigPreflightDomains(report *ConfigPreflightReport, routes []*model.ProxyRoute) {
	seen := map[string]string{}
	duplicates := []string{}
	for _, route := range routes {
		if route == nil {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			report.add(ConfigPreflightCheck{
				Key:     "domains",
				Title:   "域名配置",
				Status:  ConfigPreflightStatusError,
				Message: fmt.Sprintf("站点 %s 域名配置无效：%v", route.Domain, err),
				RouteID: route.ID,
				Site:    route.SiteName,
			})
			continue
		}
		for _, domain := range domains {
			normalized := strings.ToLower(strings.TrimSpace(domain))
			if normalized == "" {
				continue
			}
			owner := route.SiteName
			if strings.TrimSpace(owner) == "" {
				owner = route.Domain
			}
			if previousOwner, ok := seen[normalized]; ok {
				duplicates = append(duplicates, fmt.Sprintf("%s（%s / %s）", normalized, previousOwner, owner))
				continue
			}
			seen[normalized] = owner
		}
	}
	if len(duplicates) > 0 {
		sort.Strings(duplicates)
		report.add(ConfigPreflightCheck{
			Key:     "domains",
			Title:   "域名唯一性",
			Status:  ConfigPreflightStatusError,
			Message: "存在重复域名，发布后 server_name 会冲突。",
			Details: duplicates,
		})
		return
	}
	report.add(ConfigPreflightCheck{
		Key:     "domains",
		Title:   "域名唯一性",
		Status:  ConfigPreflightStatusPass,
		Message: "启用站点域名未发现重复。",
	})
}

func checkConfigPreflightCertificates(report *ConfigPreflightReport, routes []*model.ProxyRoute, context proxyRouteTLSCertificateLoader) {
	checked := 0
	for _, route := range routes {
		if route == nil || !route.EnableHTTPS {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			report.add(ConfigPreflightCheck{
				Key:     "certificates",
				Title:   "证书覆盖",
				Status:  ConfigPreflightStatusError,
				Message: fmt.Sprintf("站点 %s 域名配置无效，无法校验证书。", route.Domain),
				RouteID: route.ID,
				Site:    route.SiteName,
			})
			continue
		}
		certIDs, err := decodeStoredCertIDs(route.CertIDs, route.CertID)
		if err != nil || len(certIDs) == 0 {
			report.add(ConfigPreflightCheck{
				Key:     "certificates",
				Title:   "证书覆盖",
				Status:  ConfigPreflightStatusError,
				Message: fmt.Sprintf("站点 %s 已启用 HTTPS 但未配置证书。", route.Domain),
				RouteID: route.ID,
				Site:    route.SiteName,
			})
			continue
		}
		domainCertIDs, err := resolveProxyRouteDomainCertIDsWithContext(route, domains, certIDs, context)
		if err != nil {
			report.add(ConfigPreflightCheck{
				Key:     "certificates",
				Title:   "证书覆盖",
				Status:  ConfigPreflightStatusError,
				Message: fmt.Sprintf("站点 %s 证书域名映射无效：%v", route.Domain, err),
				RouteID: route.ID,
				Site:    route.SiteName,
			})
			continue
		}
		certificates, err := loadTLSCertificatesWithContext(context, certIDs)
		if err != nil {
			report.add(ConfigPreflightCheck{
				Key:     "certificates",
				Title:   "证书覆盖",
				Status:  ConfigPreflightStatusError,
				Message: fmt.Sprintf("站点 %s 证书不存在或无法加载。", route.Domain),
				RouteID: route.ID,
				Site:    route.SiteName,
			})
			continue
		}
		byID := make(map[uint]*model.TLSCertificate, len(certificates))
		for _, certificate := range certificates {
			if certificate != nil {
				byID[certificate.ID] = certificate
			}
		}
		for index, domain := range domains {
			if index >= len(domainCertIDs) {
				report.add(ConfigPreflightCheck{
					Key:     "certificates",
					Title:   "证书覆盖",
					Status:  ConfigPreflightStatusError,
					Message: fmt.Sprintf("域名 %s 的证书映射缺失。", domain),
					RouteID: route.ID,
					Site:    route.SiteName,
					Domain:  domain,
				})
				continue
			}
			if domainCertIDs[index] == 0 {
				report.add(ConfigPreflightCheck{
					Key:     "certificates",
					Title:   "证书覆盖",
					Status:  ConfigPreflightStatusWarning,
					Message: fmt.Sprintf("域名 %s 未关联证书，发布后会保持 HTTP-only。", domain),
					RouteID: route.ID,
					Site:    route.SiteName,
					Domain:  domain,
				})
				continue
			}
			certificate := byID[domainCertIDs[index]]
			if err := validateCertificateCoverage(certificate, []string{domain}); err != nil {
				report.add(ConfigPreflightCheck{
					Key:     "certificates",
					Title:   "证书覆盖",
					Status:  ConfigPreflightStatusError,
					Message: fmt.Sprintf("证书 %d 不覆盖域名 %s。", domainCertIDs[index], domain),
					RouteID: route.ID,
					Site:    route.SiteName,
					Domain:  domain,
				})
				continue
			}
			checked++
		}
	}
	if checked == 0 {
		report.add(ConfigPreflightCheck{
			Key:     "certificates",
			Title:   "证书覆盖",
			Status:  ConfigPreflightStatusSkipped,
			Message: "没有启用 HTTPS 的站点需要校验证书。",
		})
		return
	}
	if !hasConfigPreflightError(report, "certificates") {
		report.add(ConfigPreflightCheck{
			Key:     "certificates",
			Title:   "证书覆盖",
			Status:  ConfigPreflightStatusPass,
			Message: fmt.Sprintf("已校验 %d 个 HTTPS 域名的证书覆盖。", checked),
		})
	}
}

func checkConfigPreflightOrigins(report *ConfigPreflightReport, routes []*model.ProxyRoute, context proxyRouteTLSCertificateLoader) {
	checked := 0
	for _, route := range routes {
		if route == nil {
			continue
		}
		upstreams, err := proxyRouteEffectiveUpstreams(route, context)
		if err != nil {
			report.add(ConfigPreflightCheck{
				Key:     "origins",
				Title:   "源站配置",
				Status:  ConfigPreflightStatusError,
				Message: fmt.Sprintf("站点 %s 源站配置无效：%v", route.Domain, err),
				RouteID: route.ID,
				Site:    route.SiteName,
			})
			continue
		}
		if err := checkConfigPreflightUpstreamSet(route, "站点源站", upstreams); err != nil {
			report.add(ConfigPreflightCheck{
				Key:     "origins",
				Title:   "源站配置",
				Status:  ConfigPreflightStatusError,
				Message: err.Error(),
				RouteID: route.ID,
				Site:    route.SiteName,
			})
			continue
		}
		checked += len(upstreams)
		configs, err := loadProxyRouteRuleConfigsWithContext(context, route)
		if err != nil {
			report.add(ConfigPreflightCheck{
				Key:     "origins",
				Title:   "路径规则源站",
				Status:  ConfigPreflightStatusError,
				Message: fmt.Sprintf("站点 %s 路径规则加载失败：%v", route.Domain, err),
				RouteID: route.ID,
				Site:    route.SiteName,
			})
			continue
		}
		for _, config := range configs {
			if len(config.Upstreams) == 0 {
				continue
			}
			if err := checkConfigPreflightUpstreamSet(route, "路径规则源站", config.Upstreams); err != nil {
				report.add(ConfigPreflightCheck{
					Key:     "origins",
					Title:   "路径规则源站",
					Status:  ConfigPreflightStatusError,
					Message: err.Error(),
					RouteID: route.ID,
					Site:    route.SiteName,
				})
				continue
			}
			checked += len(config.Upstreams)
		}
	}
	if checked == 0 {
		report.add(ConfigPreflightCheck{
			Key:     "origins",
			Title:   "源站配置",
			Status:  ConfigPreflightStatusSkipped,
			Message: "没有启用站点源站需要检查。",
		})
		return
	}
	if !hasConfigPreflightError(report, "origins") {
		report.add(ConfigPreflightCheck{
			Key:     "origins",
			Title:   "源站配置",
			Status:  ConfigPreflightStatusPass,
			Message: fmt.Sprintf("已完成 %d 个源站 URL 的静态安全检查。", checked),
		})
	}
}

func checkConfigPreflightUpstreamSet(route *model.ProxyRoute, label string, upstreams []string) error {
	if len(upstreams) == 0 {
		return errors.New(label + "不能为空。")
	}
	var scheme string
	for _, upstream := range upstreams {
		if err := validateOriginURL(strings.TrimSpace(upstream)); err != nil {
			return fmt.Errorf("%s %s 无效：%w", label, upstream, err)
		}
		parsed, err := url.ParseRequestURI(upstream)
		if err != nil {
			return fmt.Errorf("%s %s 无效：%w", label, upstream, err)
		}
		if len(upstreams) > 1 {
			if parsed.Path != "" && parsed.Path != "/" {
				return fmt.Errorf("站点 %s 多源站不支持 path/query/fragment。", route.Domain)
			}
			if parsed.RawQuery != "" || parsed.Fragment != "" {
				return fmt.Errorf("站点 %s 多源站不支持 path/query/fragment。", route.Domain)
			}
		}
		if scheme == "" {
			scheme = parsed.Scheme
			continue
		}
		if parsed.Scheme != scheme {
			return fmt.Errorf("站点 %s 多源站必须使用相同 scheme。", route.Domain)
		}
	}
	return nil
}

func checkConfigPreflightCachePath(report *ConfigPreflightReport, cfg openRestyConfigSnapshot) {
	if !cfg.CacheEnabled {
		report.add(ConfigPreflightCheck{
			Key:     "cache_path",
			Title:   "缓存目录",
			Status:  ConfigPreflightStatusSkipped,
			Message: "全局缓存基础设施未启用。",
		})
		return
	}
	cachePath := strings.TrimSpace(cfg.CachePath)
	if cachePath == "" {
		report.add(ConfigPreflightCheck{
			Key:     "cache_path",
			Title:   "缓存目录",
			Status:  ConfigPreflightStatusError,
			Message: "已启用缓存基础设施，但缓存目录为空。",
		})
		return
	}
	cleaned := filepath.Clean(cachePath)
	if cleaned == "." || cleaned == "/" || cleaned == `\` || strings.Contains(cleaned, "..") {
		report.add(ConfigPreflightCheck{
			Key:     "cache_path",
			Title:   "缓存目录",
			Status:  ConfigPreflightStatusError,
			Message: "缓存目录不安全，请使用明确的专用目录。",
		})
		return
	}
	report.add(ConfigPreflightCheck{
		Key:     "cache_path",
		Title:   "缓存目录",
		Status:  ConfigPreflightStatusPass,
		Message: "缓存目录通过基础安全检查。",
	})
}

func checkConfigPreflightTemplate(report *ConfigPreflightReport) {
	if err := ValidateOpenRestyMainConfigTemplate(common.OpenRestyMainConfigTemplate); err != nil {
		report.add(ConfigPreflightCheck{
			Key:     "template",
			Title:   "OpenResty 模板占位符",
			Status:  ConfigPreflightStatusError,
			Message: err.Error(),
		})
		return
	}
	report.add(ConfigPreflightCheck{
		Key:     "template",
		Title:   "OpenResty 模板占位符",
		Status:  ConfigPreflightStatusPass,
		Message: "主配置模板保留了必需占位符。",
	})
}

func checkConfigPreflightNodePools(report *ConfigPreflightReport, routes []*model.ProxyRoute) {
	nodes, err := model.ListNodes()
	if err != nil {
		report.add(ConfigPreflightCheck{
			Key:     "node_pools",
			Title:   "节点池",
			Status:  ConfigPreflightStatusError,
			Message: fmt.Sprintf("节点池读取失败：%v", err),
		})
		return
	}
	nodePools := map[string]struct{}{normalizeNodePoolName("default"): {}}
	for _, node := range nodes {
		pool := normalizeNodePoolName(node.PoolName)
		if pool != "" {
			nodePools[pool] = struct{}{}
		}
	}
	missing := []string{}
	for _, route := range routes {
		pools, err := configVersionRouteTargetPools(route)
		if err != nil {
			report.add(ConfigPreflightCheck{
				Key:     "node_pools",
				Title:   "节点池",
				Status:  ConfigPreflightStatusError,
				Message: fmt.Sprintf("站点 %s 节点池配置无效：%v", route.Domain, err),
				RouteID: route.ID,
				Site:    route.SiteName,
			})
			continue
		}
		for _, pool := range pools {
			if _, ok := nodePools[normalizeNodePoolName(pool)]; !ok {
				missing = append(missing, pool)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		report.add(ConfigPreflightCheck{
			Key:     "node_pools",
			Title:   "节点池",
			Status:  ConfigPreflightStatusWarning,
			Message: "部分站点引用的节点池当前没有节点，发布会生成配置但不会下发到对应节点。",
			Details: dedupeStringSlice(missing),
		})
		return
	}
	report.add(ConfigPreflightCheck{
		Key:     "node_pools",
		Title:   "节点池",
		Status:  ConfigPreflightStatusPass,
		Message: "站点引用的节点池已存在。",
	})
}

func checkConfigPreflightActiveProbes(report *ConfigPreflightReport) {
	report.add(ConfigPreflightCheck{
		Key:     "origin_active_probe",
		Title:   "源站连通与 TLS 主动探测",
		Status:  ConfigPreflightStatusSkipped,
		Message: "当前发布路径执行静态公网地址、证书和渲染检查；主动 TCP/TLS 探测可在后续异步检查任务中启用，避免发布请求被外部网络阻塞。",
	})
}

func hasConfigPreflightError(report *ConfigPreflightReport, key string) bool {
	if report == nil {
		return false
	}
	for _, check := range report.Checks {
		if check.Key == key && check.Status == ConfigPreflightStatusError {
			return true
		}
	}
	return false
}

func dedupeStringSlice(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
