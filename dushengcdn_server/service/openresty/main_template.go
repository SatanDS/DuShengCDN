package openresty

import (
	"dushengcdn/common"
	"errors"
	"fmt"
	"strings"
)

var RequiredMainConfigTemplatePlaceholders = []string{
	"{{OpenRestyWorkerProcesses}}",
	"{{OpenRestyWorkerConnections}}",
	"{{OpenRestyWorkerRlimitNofile}}",
	"{{OpenRestyConnectionUpgradeMap}}",
	"{{OpenRestyDefaultServerBlock}}",
	"{{OpenRestyAccessLogPath}}",
	"{{OpenRestyEventsUseDirective}}",
	"{{OpenRestyEventsMultiAcceptDirective}}",
	"{{OpenRestyKeepaliveTimeout}}",
	"{{OpenRestyKeepaliveRequests}}",
	"{{OpenRestyClientHeaderTimeout}}",
	"{{OpenRestyClientBodyTimeout}}",
	"{{OpenRestyClientMaxBodySize}}",
	"{{OpenRestyLargeClientHeaderBuffers}}",
	"{{OpenRestySendTimeout}}",
	"{{OpenRestyProxyConnectTimeout}}",
	"{{OpenRestyProxySendTimeout}}",
	"{{OpenRestyProxyReadTimeout}}",
	"{{OpenRestyProxyRequestBuffering}}",
	"{{OpenRestyProxyBuffering}}",
	"{{OpenRestyProxyBuffers}}",
	"{{OpenRestyProxyBufferSize}}",
	"{{OpenRestyProxyBusyBuffersSize}}",
	"{{OpenRestyGzip}}",
	"{{OpenRestyGzipMinLength}}",
	"{{OpenRestyGzipCompLevel}}",
	"{{OpenRestyCacheBlock}}",
	"{{OpenRestyRouteConfigInclude}}",
}

func RenderMainConfig(cfg ConfigSnapshot) string {
	templateText := common.OpenRestyMainConfigTemplate
	if strings.TrimSpace(templateText) == "" {
		templateText = DefaultMainConfigTemplate()
	}
	return RenderMainConfigTemplate(templateText, cfg)
}

func ValidateMainConfigTemplate(templateText string) error {
	trimmed := strings.TrimSpace(templateText)
	if trimmed == "" {
		return errors.New("OpenRestyMainConfigTemplate 涓嶈兘涓虹┖")
	}
	for _, placeholder := range RequiredMainConfigTemplatePlaceholders {
		if !strings.Contains(trimmed, placeholder) {
			return fmt.Errorf("OpenRestyMainConfigTemplate 蹇呴』淇濈暀鍗犱綅绗?%s", placeholder)
		}
	}
	return nil
}

func DefaultMainConfigTemplate() string {
	return common.OpenRestyMainConfigTemplate
}

func RenderMainConfigTemplate(templateText string, cfg ConfigSnapshot) string {
	replacer := strings.NewReplacer(
		"{{OpenRestyWorkerProcesses}}", cfg.WorkerProcesses,
		"{{OpenRestyWorkerConnections}}", fmt.Sprintf("%d", cfg.WorkerConnections),
		"{{OpenRestyWorkerRlimitNofile}}", fmt.Sprintf("%d", cfg.WorkerRlimitNofile),
		"{{OpenRestyConnectionUpgradeMap}}", renderConnectionUpgradeMap(),
		"{{OpenRestyDefaultServerBlock}}", renderDefaultServerBlock(),
		"{{OpenRestyAccessLogPath}}", AccessLogPlaceholder,
		"{{OpenRestyEventsUseDirective}}", renderTemplateDirective(cfg.EventsUse != "", fmt.Sprintf("use %s;", cfg.EventsUse)),
		"{{OpenRestyEventsMultiAcceptDirective}}", renderTemplateDirective(cfg.EventsMultiAcceptEnabled, "multi_accept on;"),
		"{{OpenRestyKeepaliveTimeout}}", fmt.Sprintf("%d", cfg.KeepaliveTimeout),
		"{{OpenRestyKeepaliveRequests}}", fmt.Sprintf("%d", cfg.KeepaliveRequests),
		"{{OpenRestyClientHeaderTimeout}}", fmt.Sprintf("%d", cfg.ClientHeaderTimeout),
		"{{OpenRestyClientBodyTimeout}}", fmt.Sprintf("%d", cfg.ClientBodyTimeout),
		"{{OpenRestyClientMaxBodySize}}", cfg.ClientMaxBodySize,
		"{{OpenRestyLargeClientHeaderBuffers}}", cfg.LargeClientHeaderBuffers,
		"{{OpenRestySendTimeout}}", fmt.Sprintf("%d", cfg.SendTimeout),
		"{{OpenRestyProxyConnectTimeout}}", fmt.Sprintf("%d", cfg.ProxyConnectTimeout),
		"{{OpenRestyProxySendTimeout}}", fmt.Sprintf("%d", cfg.ProxySendTimeout),
		"{{OpenRestyProxyReadTimeout}}", fmt.Sprintf("%d", cfg.ProxyReadTimeout),
		"{{OpenRestyProxyRequestBuffering}}", onOff(cfg.ProxyRequestBuffering),
		"{{OpenRestyProxyBuffering}}", onOff(cfg.ProxyBufferingEnabled),
		"{{OpenRestyProxyBuffers}}", cfg.ProxyBuffers,
		"{{OpenRestyProxyBufferSize}}", cfg.ProxyBufferSize,
		"{{OpenRestyProxyBusyBuffersSize}}", cfg.ProxyBusyBuffersSize,
		"{{OpenRestyGzip}}", onOff(cfg.GzipEnabled),
		"{{OpenRestyGzipMinLength}}", fmt.Sprintf("%d", cfg.GzipMinLength),
		"{{OpenRestyGzipCompLevel}}", fmt.Sprintf("%d", cfg.GzipCompLevel),
		"{{OpenRestyResolverDirective}}", renderOpenRestyResolverDirective(cfg.Resolvers),
		"{{OpenRestyCacheBlock}}", renderOpenRestyCacheTemplateBlock(cfg),
		"{{OpenRestyRouteConfigInclude}}", RouteConfigPlaceholder,
	)
	return replacer.Replace(templateText)
}

func renderTemplateDirective(enabled bool, statement string) string {
	if !enabled {
		return ""
	}
	return fmt.Sprintf("    %s\n", statement)
}

func renderOpenRestyResolverDirective(resolvers string) string {
	trimmed := strings.TrimSpace(resolvers)
	if trimmed != "" {
		return renderTemplateDirective(true, fmt.Sprintf("resolver %s;", trimmed))
	}
	return fmt.Sprintf("    %s\n", ResolverDirectivePlaceholder)
}

func renderOpenRestyLimitZoneBlock() string {
	return strings.Join([]string{
		"    limit_conn_zone $server_name zone=dushengcdn_conn_per_server:10m;",
		"    limit_conn_zone $binary_remote_addr zone=dushengcdn_conn_per_ip:10m;",
		"",
	}, "\n")
}

func renderConnectionUpgradeMap() string {
	return "    map $http_upgrade $connection_upgrade {\n        default upgrade;\n        ''      \"\";\n    }\n\n"
}

func renderDefaultServerBlock() string {
	return strings.Join([]string{
		"    server {",
		"        listen 80 default_server;",
		"        server_name _;",
		"        set $dushengcdn_request_reason \"\";",
		"",
		"        return 404;",
		"    }",
		"",
		"    server {",
		"        listen 443 ssl default_server;",
		"        server_name _;",
		"        set $dushengcdn_request_reason \"\";",
		"",
		"        ssl_reject_handshake on;",
		"    }",
		"",
	}, "\n")
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
