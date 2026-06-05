package service

import (
	"dushengcdn/model"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCleanupConfigVersionsDeletesOnlyOldInactiveRows(t *testing.T) {
	setupServiceTestDB(t)

	for i := 1; i <= 5; i++ {
		version := &model.ConfigVersion{
			Version:          fmt.Sprintf("2026060500000%d", i),
			SnapshotJSON:     "{}",
			MainConfig:       "main",
			RenderedConfig:   "rendered",
			SupportFilesJSON: "[]",
			Checksum:         fmt.Sprintf("checksum-%d", i),
			IsActive:         i == 1,
			CreatedBy:        "root",
		}
		if err := model.DB.Create(version).Error; err != nil {
			t.Fatalf("seed config version %d: %v", i, err)
		}
	}

	deleted, err := CleanupConfigVersions(3)
	if err != nil {
		t.Fatalf("CleanupConfigVersions failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 old inactive version deleted, got %d", deleted)
	}

	remaining := map[string]bool{}
	var versions []model.ConfigVersion
	if err := model.DB.Select("version", "is_active").Order("id asc").Find(&versions).Error; err != nil {
		t.Fatalf("list remaining config versions: %v", err)
	}
	for _, version := range versions {
		remaining[version.Version] = version.IsActive
	}
	if len(remaining) != 4 {
		t.Fatalf("expected 4 remaining versions, got %#v", remaining)
	}
	if active, ok := remaining["20260605000001"]; !ok || !active {
		t.Fatalf("expected old active version to be preserved, got %#v", remaining)
	}
	if _, ok := remaining["20260605000002"]; ok {
		t.Fatalf("expected oldest inactive version outside keep window to be deleted, got %#v", remaining)
	}
	for _, version := range []string{"20260605000003", "20260605000004", "20260605000005"} {
		if _, ok := remaining[version]; !ok {
			t.Fatalf("expected recent version %s to be preserved, got %#v", version, remaining)
		}
	}
}

func TestRenderRouteConfigBatchesSharedCertificateLoading(t *testing.T) {
	setupServiceTestDB(t)

	certPEM, keyPEM := generateCertificatePair(t, []string{"app.example.com", "www.example.com"})
	certificate, err := CreateTLSCertificate(TLSCertificateInput{
		Name:    "shared-example",
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	})
	if err != nil {
		t.Fatalf("CreateTLSCertificate failed: %v", err)
	}

	routes := []*model.ProxyRoute{
		{
			ID:                         1,
			SiteName:                   "app",
			Domain:                     "app.example.com",
			Domains:                    mustJSON(t, []string{"app.example.com"}),
			OriginURL:                  "https://origin.internal",
			Upstreams:                  mustJSON(t, []string{"https://origin.internal"}),
			EnableHTTPS:                true,
			CertID:                     &certificate.ID,
			CertIDs:                    mustJSON(t, []uint{certificate.ID}),
			DomainCertIDs:              "[]",
			RedirectHTTP:               true,
			CustomHeaders:              "[]",
			CacheRules:                 "[]",
			RegionRestrictionMode:      proxyRouteRegionModeBlock,
			RegionRestrictionCountries: "[]",
			WAFMode:                    proxyRouteWAFModeBlock,
			CCMode:                     proxyRouteCCModeBlock,
		},
		{
			ID:                         2,
			SiteName:                   "www",
			Domain:                     "www.example.com",
			Domains:                    mustJSON(t, []string{"www.example.com"}),
			OriginURL:                  "https://origin.internal",
			Upstreams:                  mustJSON(t, []string{"https://origin.internal"}),
			EnableHTTPS:                true,
			CertID:                     &certificate.ID,
			CertIDs:                    mustJSON(t, []uint{certificate.ID}),
			DomainCertIDs:              "[]",
			RedirectHTTP:               true,
			CustomHeaders:              "[]",
			CacheRules:                 "[]",
			RegionRestrictionMode:      proxyRouteRegionModeBlock,
			RegionRestrictionCountries: "[]",
			WAFMode:                    proxyRouteWAFModeBlock,
			CCMode:                     proxyRouteCCModeBlock,
		},
	}

	var loadCalls int
	routeConfig, supportFiles, err := renderRouteConfigWithQueries(routes, buildOpenRestyConfigSnapshot(), routeConfigQueries{
		ListTLSCertificatesByIDs: func(ids []uint) ([]*model.TLSCertificate, error) {
			loadCalls++
			if !reflect.DeepEqual(ids, []uint{certificate.ID}) {
				t.Fatalf("expected one shared certificate id to be batched, got %#v", ids)
			}
			return model.ListTLSCertificatesByIDs(ids)
		},
	})
	if err != nil {
		t.Fatalf("renderRouteConfigWithQueries failed: %v", err)
	}
	if loadCalls != 1 {
		t.Fatalf("expected one batched certificate load, got %d", loadCalls)
	}
	if strings.Count(routeConfig, "ssl_certificate __DUSHENGCDN_CERT_DIR__/") != 2 {
		t.Fatalf("expected both HTTPS servers to reference certificates, got %s", routeConfig)
	}

	files := map[string]SupportFile{}
	for _, file := range supportFiles {
		files[file.Path] = file
	}
	if len(files) != 2 {
		t.Fatalf("expected shared certificate support files to be deduplicated, got %#v", supportFiles)
	}
	if _, ok := files[certificateCertFileName(certificate.ID)]; !ok {
		t.Fatalf("expected certificate file %s, got %#v", certificateCertFileName(certificate.ID), files)
	}
	if _, ok := files[certificateKeyFileName(certificate.ID)]; !ok {
		t.Fatalf("expected key file %s, got %#v", certificateKeyFileName(certificate.ID), files)
	}
}

func TestBuildSnapshotRoutesBatchesLegacyCertificateInference(t *testing.T) {
	setupServiceTestDB(t)

	certPEM, keyPEM := generateCertificatePair(t, []string{"*.example.com"})
	certificate, err := CreateTLSCertificate(TLSCertificateInput{
		Name:    "snapshot-wildcard",
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	})
	if err != nil {
		t.Fatalf("CreateTLSCertificate failed: %v", err)
	}

	routes := []*model.ProxyRoute{}
	for index, domain := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		routes = append(routes, &model.ProxyRoute{
			ID:                         uint(index + 1),
			SiteName:                   strings.TrimSuffix(domain, ".example.com"),
			Domain:                     domain,
			Domains:                    mustJSON(t, []string{domain}),
			OriginURL:                  "https://origin.internal",
			Upstreams:                  mustJSON(t, []string{"https://origin.internal"}),
			EnableHTTPS:                true,
			CertID:                     &certificate.ID,
			CertIDs:                    mustJSON(t, []uint{certificate.ID}),
			DomainCertIDs:              "[]",
			CustomHeaders:              "[]",
			CacheRules:                 "[]",
			RegionRestrictionMode:      proxyRouteRegionModeBlock,
			RegionRestrictionCountries: "[]",
			WAFMode:                    proxyRouteWAFModeBlock,
			CCMode:                     proxyRouteCCModeBlock,
		})
	}

	previousQueries := defaultRouteConfigQueries
	var loadCalls int
	defaultRouteConfigQueries = routeConfigQueries{
		ListTLSCertificatesByIDs: func(ids []uint) ([]*model.TLSCertificate, error) {
			loadCalls++
			if !reflect.DeepEqual(ids, []uint{certificate.ID}) {
				t.Fatalf("expected one shared certificate id to be batched, got %#v", ids)
			}
			return model.ListTLSCertificatesByIDs(ids)
		},
	}
	t.Cleanup(func() {
		defaultRouteConfigQueries = previousQueries
	})

	snapshotRoutes, err := buildSnapshotRoutes(routes)
	if err != nil {
		t.Fatalf("buildSnapshotRoutes failed: %v", err)
	}
	if loadCalls != 1 {
		t.Fatalf("expected one batched certificate load, got %d", loadCalls)
	}
	if len(snapshotRoutes) != len(routes) {
		t.Fatalf("expected %d snapshot routes, got %d", len(routes), len(snapshotRoutes))
	}
	for _, route := range snapshotRoutes {
		if len(route.DomainCertIDs) != 1 || route.DomainCertIDs[0] != certificate.ID {
			t.Fatalf("expected inferred domain certificate id for snapshot route %+v", route)
		}
	}
}

func TestBuildCurrentConfigBundleSharesCertificateContext(t *testing.T) {
	setupServiceTestDB(t)

	certPEM, keyPEM := generateCertificatePair(t, []string{"*.example.com"})
	certificate, err := CreateTLSCertificate(TLSCertificateInput{
		Name:    "bundle-wildcard",
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	})
	if err != nil {
		t.Fatalf("CreateTLSCertificate failed: %v", err)
	}

	for index, domain := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		route := &model.ProxyRoute{
			ID:                         uint(index + 1),
			SiteName:                   strings.TrimSuffix(domain, ".example.com"),
			Domain:                     domain,
			Domains:                    mustJSON(t, []string{domain}),
			OriginURL:                  "https://origin.internal",
			Upstreams:                  mustJSON(t, []string{"https://origin.internal"}),
			NodePool:                   "default",
			Enabled:                    true,
			EnableHTTPS:                true,
			CertID:                     &certificate.ID,
			CertIDs:                    mustJSON(t, []uint{certificate.ID}),
			DomainCertIDs:              "[]",
			RedirectHTTP:               true,
			CustomHeaders:              "[]",
			CacheRules:                 "[]",
			PoWConfig:                  "{}",
			WAFMode:                    proxyRouteWAFModeBlock,
			WAFConfig:                  "{}",
			CCMode:                     proxyRouteCCModeBlock,
			CCConfig:                   "{}",
			RegionRestrictionMode:      proxyRouteRegionModeBlock,
			RegionRestrictionCountries: "[]",
			DNSRecordType:              "A",
			DNSTargetCount:             1,
			DNSScheduleMode:            "healthy",
			DNSTTL:                     1,
			DNSProviderMode:            DNSProviderModeCloudflare,
			DNSRecordIDs:               "{}",
			DDOSProtectionMode:         DDOSProtectionModeOff,
			DDOSProtectionProvider:     DDOSProtectionProviderCloudflare,
			GSLBPolicy:                 "{}",
		}
		if err := route.Insert(); err != nil {
			t.Fatalf("insert proxy route: %v", err)
		}
	}

	previousQueries := defaultRouteConfigQueries
	var loadCalls int
	defaultRouteConfigQueries = routeConfigQueries{
		ListTLSCertificatesByIDs: func(ids []uint) ([]*model.TLSCertificate, error) {
			loadCalls++
			if !reflect.DeepEqual(ids, []uint{certificate.ID}) {
				t.Fatalf("expected one shared certificate id to be batched, got %#v", ids)
			}
			return model.ListTLSCertificatesByIDs(ids)
		},
	}
	t.Cleanup(func() {
		defaultRouteConfigQueries = previousQueries
	})

	bundle, err := buildCurrentConfigBundle(true)
	if err != nil {
		t.Fatalf("buildCurrentConfigBundle failed: %v", err)
	}
	if loadCalls != 1 {
		t.Fatalf("expected snapshot and route rendering to share one certificate load, got %d", loadCalls)
	}
	if len(bundle.SnapshotRoutes) != 3 {
		t.Fatalf("expected 3 snapshot routes, got %d", len(bundle.SnapshotRoutes))
	}
	if strings.Count(bundle.RouteConfig, "ssl_certificate __DUSHENGCDN_CERT_DIR__/") != 3 {
		t.Fatalf("expected rendered config to include 3 HTTPS server blocks, got %s", bundle.RouteConfig)
	}
}
