package openresty

import (
	"fmt"
	"strings"
)

func renderHTTPProxyServer(serverNames string, content renderedRouteServerContent) string {
	return fmt.Sprintf("server {\n    listen 80;\n    server_name %s;\n    set $dushengcdn_request_reason \"\";\n%s}\n\n", serverNames, content.String())
}

func renderHTTPRedirectServer(serverNames string, regionConfig routeRegionRestrictionConfig, wafConfig routeWAFConfig, ccConfig routeCCConfig) string {
	return fmt.Sprintf("server {\n    listen 80;\n    server_name %s;\n    set $dushengcdn_request_reason \"\";\n%s\n    return 301 https://$host$request_uri;\n}\n\n", serverNames, renderRegionRestrictionBlock(regionConfig, wafConfig, ccConfig))
}

func renderHTTPSServer(serverNames string, certificateID uint, content renderedRouteServerContent) string {
	certPath := fmt.Sprintf("%s/%s", CertDirPlaceholder, CertFileName(certificateID))
	keyPath := fmt.Sprintf("%s/%s", CertDirPlaceholder, KeyFileName(certificateID))
	return fmt.Sprintf("server {\n    listen 443 ssl;\n    http2 on;\n    server_name %s;\n    ssl_certificate %s;\n    ssl_certificate_key %s;\n    set $dushengcdn_request_reason \"\";\n%s}\n\n", serverNames, certPath, keyPath, content.String())
}

func renderRouteServerContent(locationSet renderedRouteLocationSet, cfg ConfigSnapshot) renderedRouteServerContent {
	powEnabled := routeLocationSetPowEnabled(locationSet.locations)
	return renderedRouteServerContent{
		accessBlock:       renderRouteLocationServerAccessBlock(locationSet.locations),
		powLocationBlocks: renderPowLocationBlocks(powEnabled),
		locationBlocks:    renderRouteLocationBlocks(locationSet.locations, cfg),
		powStaticBlock:    renderPowStaticLocationBlock(powEnabled),
	}
}

func (content renderedRouteServerContent) String() string {
	return content.accessBlock + content.powLocationBlocks + content.locationBlocks + content.powStaticBlock
}

func renderRouteLocationServerAccessBlock(locations []renderedRouteLocation) string {
	for _, location := range locations {
		config := location.config
		if config.powEnabled || config.wafConfig.Enabled || config.ccConfig.Enabled || (config.regionConfig.Enabled && len(config.regionConfig.Countries) > 0) {
			return renderUnifiedAccessBlock(true)
		}
	}
	return ""
}

func routeLocationSetPowEnabled(locations []renderedRouteLocation) bool {
	for _, location := range locations {
		if location.config.powEnabled {
			return true
		}
	}
	return false
}

func renderServerNames(domains []string) string {
	return strings.Join(domains, " ")
}
