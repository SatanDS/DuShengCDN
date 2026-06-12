package service

import (
	"context"
	"dushengcdn/model"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
)

func TestCleanupConfigVersionsDeletesOnlyOldInactiveRows(t *testing.T) {
	setupServiceTestDB(t)

	seededVersionIDs := map[string]uint{}
	for i := 1; i <= 5; i++ {
		versionNumber := fmt.Sprintf("2026060500000%d", i)
		version := &model.ConfigVersion{
			Version:          versionNumber,
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
		seededVersionIDs[versionNumber] = version.ID
		artifact := &model.ConfigVersionArtifact{
			ConfigVersionID:     version.ID,
			PoolName:            "default",
			Checksum:            version.Checksum,
			MainConfigChecksum:  fmt.Sprintf("main-checksum-%d", i),
			RouteConfigChecksum: fmt.Sprintf("route-checksum-%d", i),
			RenderedConfig:      "server {}",
			SupportFilesJSON:    "[]",
			RouteCount:          1,
		}
		if err := model.DB.Create(artifact).Error; err != nil {
			t.Fatalf("seed config version artifact %d: %v", i, err)
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

	var artifactCount int64
	if err := model.DB.Model(&model.ConfigVersionArtifact{}).Count(&artifactCount).Error; err != nil {
		t.Fatalf("count remaining config version artifacts: %v", err)
	}
	if artifactCount != 4 {
		t.Fatalf("expected artifacts for remaining versions only, got %d", artifactCount)
	}
	var deletedArtifactCount int64
	if err := model.DB.Model(&model.ConfigVersionArtifact{}).
		Where("config_version_id = ?", seededVersionIDs["20260605000002"]).
		Count(&deletedArtifactCount).Error; err != nil {
		t.Fatalf("count deleted config version artifacts: %v", err)
	}
	if deletedArtifactCount != 0 {
		t.Fatalf("expected deleted version artifacts to be removed, got %d", deletedArtifactCount)
	}
}

func TestRedactRenderedConfigForAdminScrubsBasicAuthHash(t *testing.T) {
	rendered := `server {
    location / {
        rewrite_by_lua_block {
            local expected_hash = "abcdef123456"
        }
        proxy_set_header Authorization "Bearer origin-token";
        proxy_set_header X-API-Key "origin-api-key";
        proxy_set_header X-Safe "safe-value";
    }
}`

	redacted := redactRenderedConfigForAdmin(rendered)
	for _, leaked := range []string{"abcdef123456", "origin-token", "origin-api-key"} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("expected %q to be redacted from %s", leaked, redacted)
		}
	}
	if !strings.Contains(redacted, `proxy_set_header X-Safe "safe-value";`) {
		t.Fatalf("expected non-sensitive header to remain, got %s", redacted)
	}
}

func TestPublishConfigVersionCreatesArtifactsPerNodePool(t *testing.T) {
	setupServiceTestDB(t)

	for _, input := range []NodeInput{
		{Name: "default-edge", PoolName: "default"},
		{Name: "hk-edge", PoolName: "hk"},
		{Name: "eu-edge", PoolName: "eu"},
		{Name: "idle-edge", PoolName: "idle"},
	} {
		if _, err := CreateNode(input); err != nil {
			t.Fatalf("CreateNode(%s) failed: %v", input.Name, err)
		}
	}

	gslbPolicy := defaultGSLBPolicy("hk", 1, "weighted", 60)
	gslbPolicy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "hk", Weight: 100, Enabled: true},
		{Name: "eu", Weight: 100, Enabled: true},
		{Name: "disabled", Weight: 100, Enabled: false},
	}
	seedConfigVersionArtifactTestRoute(t, &model.ProxyRoute{
		SiteName:                   "app-hk",
		Domain:                     "app-hk.example.com",
		Domains:                    mustJSON(t, []string{"app-hk.example.com"}),
		OriginURL:                  "https://origin-hk.internal",
		Upstreams:                  mustJSON(t, []string{"https://origin-hk.internal"}),
		NodePool:                   "hk",
		Enabled:                    true,
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
		GSLBEnabled:                true,
		GSLBPolicy:                 mustJSON(t, gslbPolicy),
		DDOSProtectionMode:         DDOSProtectionModeOff,
		DDOSProtectionProvider:     DDOSProtectionProviderCloudflare,
	})
	seedConfigVersionArtifactTestRoute(t, &model.ProxyRoute{
		SiteName:                   "ddos",
		Domain:                     "ddos.example.com",
		Domains:                    mustJSON(t, []string{"ddos.example.com"}),
		OriginURL:                  "https://origin-default.internal",
		Upstreams:                  mustJSON(t, []string{"https://origin-default.internal"}),
		NodePool:                   "default",
		Enabled:                    true,
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
		GSLBPolicy:                 "{}",
		DDOSProtectionMode:         DDOSProtectionModeAuto,
		DDOSProtectionProvider:     DDOSProtectionProviderCustom,
		DDOSProtectionTarget:       "eu",
	})

	release, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	artifacts, err := model.ListConfigVersionArtifacts(release.Version.ID)
	if err != nil {
		t.Fatalf("ListConfigVersionArtifacts failed: %v", err)
	}
	byPool := map[string]*model.ConfigVersionArtifact{}
	for _, artifact := range artifacts {
		byPool[artifact.PoolName] = artifact
	}
	for _, poolName := range []string{"default", "hk", "eu", "idle"} {
		if byPool[poolName] == nil {
			t.Fatalf("expected artifact for pool %s, got %#v", poolName, byPool)
		}
		if byPool[poolName].Checksum == "" || byPool[poolName].MainConfigChecksum == "" || byPool[poolName].RouteConfigChecksum == "" {
			t.Fatalf("expected artifact checksums for pool %s, got %+v", poolName, byPool[poolName])
		}
	}
	if _, ok := byPool["disabled"]; ok {
		t.Fatalf("did not expect artifact for disabled GSLB pool, got %#v", byPool)
	}
	if byPool["hk"].RouteCount != 1 {
		t.Fatalf("expected hk artifact to contain one route, got %d", byPool["hk"].RouteCount)
	}
	if byPool["eu"].RouteCount != 2 {
		t.Fatalf("expected eu artifact to include GSLB and DDoS target routes, got %d", byPool["eu"].RouteCount)
	}
	if byPool["idle"].RouteCount != 0 {
		t.Fatalf("expected idle pool artifact to be empty, got %d", byPool["idle"].RouteCount)
	}
	if !strings.Contains(byPool["hk"].RenderedConfig, "app-hk.example.com") || strings.Contains(byPool["hk"].RenderedConfig, "ddos.example.com") {
		t.Fatalf("expected hk artifact to contain only hk route config, got %s", byPool["hk"].RenderedConfig)
	}
	if !strings.Contains(byPool["eu"].RenderedConfig, "app-hk.example.com") || !strings.Contains(byPool["eu"].RenderedConfig, "ddos.example.com") {
		t.Fatalf("expected eu artifact to contain GSLB and DDoS route config, got %s", byPool["eu"].RenderedConfig)
	}
	if strings.Contains(byPool["idle"].RenderedConfig, "app-hk.example.com") || strings.Contains(byPool["idle"].RenderedConfig, "ddos.example.com") {
		t.Fatalf("expected idle artifact route config to be empty, got %s", byPool["idle"].RenderedConfig)
	}
	if byPool["hk"].Checksum == byPool["idle"].Checksum {
		t.Fatalf("expected route-bearing and empty pool artifacts to have different checksums")
	}
}

func TestPublishConfigVersionDetectsPoolOnlyArtifactChanges(t *testing.T) {
	setupServiceTestDB(t)

	if _, err := CreateNode(NodeInput{Name: "default-edge", PoolName: "default"}); err != nil {
		t.Fatalf("CreateNode default failed: %v", err)
	}
	if _, err := CreateNode(NodeInput{Name: "hk-edge", PoolName: "hk"}); err != nil {
		t.Fatalf("CreateNode hk failed: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:                   "pool-only",
		Domain:                     "pool-only.example.com",
		Domains:                    mustJSON(t, []string{"pool-only.example.com"}),
		OriginURL:                  "https://origin.internal",
		Upstreams:                  mustJSON(t, []string{"https://origin.internal"}),
		NodePool:                   "default",
		Enabled:                    true,
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
		GSLBPolicy:                 "{}",
		DDOSProtectionMode:         DDOSProtectionModeOff,
		DDOSProtectionProvider:     DDOSProtectionProviderCloudflare,
	}
	seedConfigVersionArtifactTestRoute(t, route)
	firstRelease, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("initial PublishConfigVersion failed: %v", err)
	}
	route.NodePool = "hk"
	if err := route.Update(); err != nil {
		t.Fatalf("move route pool: %v", err)
	}

	changed, err := HasConfigChanges()
	if err != nil {
		t.Fatalf("HasConfigChanges failed: %v", err)
	}
	if !changed {
		t.Fatal("expected pool-only artifact change to be publishable")
	}
	diff, err := DiffConfigVersion()
	if err != nil {
		t.Fatalf("DiffConfigVersion failed: %v", err)
	}
	if !diff.RuntimeConfigChanged {
		t.Fatalf("expected pool-only artifact change to mark runtime config changed, got %+v", diff)
	}
	secondRelease, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("pool-only PublishConfigVersion failed: %v", err)
	}
	if firstRelease.Version.Checksum != secondRelease.Version.Checksum {
		t.Fatal("expected global runtime checksum to remain tied to rendered all-route config")
	}
	defaultArtifact, err := model.GetConfigVersionArtifact(secondRelease.Version.ID, "default")
	if err != nil {
		t.Fatalf("load default artifact: %v", err)
	}
	hkArtifact, err := model.GetConfigVersionArtifact(secondRelease.Version.ID, "hk")
	if err != nil {
		t.Fatalf("load hk artifact: %v", err)
	}
	if defaultArtifact.Checksum == hkArtifact.Checksum {
		t.Fatal("expected pool artifact checksums to differ after moving route pools")
	}
	if defaultArtifact.RouteCount != 0 || hkArtifact.RouteCount != 1 {
		t.Fatalf("unexpected route counts after pool move: default=%d hk=%d", defaultArtifact.RouteCount, hkArtifact.RouteCount)
	}
}

func TestHistoricalActiveConfigBackfillsPoolArtifacts(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:       "legacy-hk-node",
		Name:         "legacy-hk-node",
		IP:           "10.0.0.50",
		PoolName:     "hk",
		AgentToken:   "legacy-hk-token",
		AgentVersion: "v0.5.0",
		Status:       NodeStatusOnline,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	version := &model.ConfigVersion{
		Version:          "20260609-legacy",
		SnapshotJSON:     `{"routes":[{"domain":"legacy.example.com"}]}`,
		MainConfig:       "worker_processes auto;",
		RenderedConfig:   "server { server_name legacy.example.com; }",
		SupportFilesJSON: "[]",
		Checksum:         "legacy-global-checksum",
		IsActive:         true,
		CreatedBy:        "root",
	}
	if err := model.DB.Create(version).Error; err != nil {
		t.Fatalf("seed legacy active config version: %v", err)
	}

	config, err := GetActiveConfigForAgentNode(node)
	if err != nil {
		t.Fatalf("GetActiveConfigForAgentNode should backfill historical pool artifact: %v", err)
	}
	if config.Checksum != version.Checksum || config.RenderedConfig != version.RenderedConfig {
		t.Fatalf("expected historical pool artifact to reuse global config, got %+v", config)
	}
	artifact, err := model.GetConfigVersionArtifact(version.ID, "hk")
	if err != nil {
		t.Fatalf("expected hk compatibility artifact: %v", err)
	}
	if artifact.Checksum != version.Checksum || artifact.RouteCount != 1 {
		t.Fatalf("unexpected compatibility artifact: %+v", artifact)
	}
}

func TestEnsureConfigVersionArtifactsForPoolsBackfillsMissingPools(t *testing.T) {
	setupServiceTestDB(t)

	version := &model.ConfigVersion{
		Version:          "20260609-partial",
		SnapshotJSON:     `{"routes":[{"domain":"partial.example.com"}]}`,
		MainConfig:       "worker_processes auto;",
		RenderedConfig:   "server { server_name partial.example.com; }",
		SupportFilesJSON: "[]",
		Checksum:         "partial-checksum",
		IsActive:         true,
		CreatedBy:        "root",
	}
	if err := model.DB.Create(version).Error; err != nil {
		t.Fatalf("seed version: %v", err)
	}
	if err := model.DB.Create(&model.ConfigVersionArtifact{
		ConfigVersionID:     version.ID,
		PoolName:            "default",
		Checksum:            version.Checksum,
		MainConfigChecksum:  "main-default",
		RouteConfigChecksum: "route-default",
		RenderedConfig:      "default rendered",
		SupportFilesJSON:    "[]",
		RouteCount:          0,
	}).Error; err != nil {
		t.Fatalf("seed default artifact: %v", err)
	}

	if err := ensureConfigVersionArtifactsForPools(version, []string{"default", "hk", "eu"}); err != nil {
		t.Fatalf("ensureConfigVersionArtifactsForPools failed: %v", err)
	}

	artifacts, err := model.ListConfigVersionArtifacts(version.ID)
	if err != nil {
		t.Fatalf("ListConfigVersionArtifacts failed: %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts after backfill, got %d", len(artifacts))
	}
	pools := map[string]struct{}{}
	for _, artifact := range artifacts {
		pools[artifact.PoolName] = struct{}{}
	}
	for _, poolName := range []string{"default", "hk", "eu"} {
		if _, ok := pools[poolName]; !ok {
			t.Fatalf("expected artifact for pool %s, got %#v", poolName, pools)
		}
	}
}

func TestHasConfigChangesDoesNotBackfillMissingActiveArtifacts(t *testing.T) {
	setupServiceTestDB(t)

	seedConfigVersionArtifactTestRoute(t, &model.ProxyRoute{
		SiteName:                   "legacy-read-only",
		Domain:                     "legacy-read-only.example.com",
		Domains:                    mustJSON(t, []string{"legacy-read-only.example.com"}),
		OriginURL:                  "https://origin.internal",
		Upstreams:                  mustJSON(t, []string{"https://origin.internal"}),
		NodePool:                   "default",
		Enabled:                    true,
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
		GSLBPolicy:                 "{}",
		DDOSProtectionMode:         DDOSProtectionModeOff,
		DDOSProtectionProvider:     DDOSProtectionProviderCloudflare,
	})
	bundle, err := buildCurrentConfigBundle(true)
	if err != nil {
		t.Fatalf("buildCurrentConfigBundle failed: %v", err)
	}
	supportFilesJSON, err := json.Marshal(bundle.SupportFiles)
	if err != nil {
		t.Fatalf("marshal support files: %v", err)
	}
	version := &model.ConfigVersion{
		Version:          "20260611-legacy-read-only",
		SnapshotJSON:     bundle.SnapshotJSON,
		MainConfig:       bundle.MainConfig,
		RenderedConfig:   bundle.RouteConfig,
		SupportFilesJSON: string(supportFilesJSON),
		Checksum:         bundle.Checksum,
		IsActive:         true,
		CreatedBy:        "root",
	}
	if err := model.DB.Create(version).Error; err != nil {
		t.Fatalf("seed legacy active config version: %v", err)
	}

	changed, err := HasConfigChanges()
	if err != nil {
		t.Fatalf("HasConfigChanges failed: %v", err)
	}
	if changed {
		t.Fatal("expected legacy active config without artifacts to compare equal in read-only path")
	}
	var artifactCount int64
	if err := model.DB.Model(&model.ConfigVersionArtifact{}).Where("config_version_id = ?", version.ID).Count(&artifactCount).Error; err != nil {
		t.Fatalf("count config version artifacts: %v", err)
	}
	if artifactCount != 0 {
		t.Fatalf("expected HasConfigChanges to avoid artifact backfill writes, got %d artifacts", artifactCount)
	}
}

func seedConfigVersionArtifactTestRoute(t *testing.T, route *model.ProxyRoute) {
	t.Helper()
	if route.DomainCertIDs == "" {
		route.DomainCertIDs = "[]"
	}
	if route.DNSRecordIDs == "" {
		route.DNSRecordIDs = "{}"
	}
	if route.GSLBPolicy == "" {
		route.GSLBPolicy = "{}"
	}
	if route.DDOSProtectionMode == "" {
		route.DDOSProtectionMode = DDOSProtectionModeOff
	}
	if route.DDOSProtectionProvider == "" {
		route.DDOSProtectionProvider = DDOSProtectionProviderCloudflare
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert proxy route %s: %v", route.Domain, err)
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

func TestPreviewConfigVersionMemoizesDNSAcrossBundleRenders(t *testing.T) {
	setupServiceTestDB(t)
	previousLookup := routeUpstreamLookupIPAddr
	lookupCalls := 0
	routeUpstreamLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		lookupCalls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected publish-time lookup context to have a deadline")
		}
		return []net.IPAddr{
			{IP: net.ParseIP("93.184.216.35")},
			{IP: net.ParseIP("93.184.216.34")},
		}, nil
	}
	t.Cleanup(func() {
		routeUpstreamLookupIPAddr = previousLookup
	})

	seedConfigVersionArtifactTestRoute(t, &model.ProxyRoute{
		SiteName:                   "memoized-dns",
		Domain:                     "memoized-dns.example.com",
		Domains:                    mustJSON(t, []string{"memoized-dns.example.com"}),
		OriginURL:                  "https://origin.example.net:8443",
		Upstreams:                  mustJSON(t, []string{"https://origin.example.net:8443"}),
		Enabled:                    true,
		NodePool:                   "default",
		CustomHeaders:              "[]",
		CacheRules:                 "[]",
		RegionRestrictionMode:      proxyRouteRegionModeBlock,
		RegionRestrictionCountries: "[]",
		WAFMode:                    proxyRouteWAFModeBlock,
		CCMode:                     proxyRouteCCModeBlock,
	})

	preview, err := PreviewConfigVersion()
	if err != nil {
		t.Fatalf("PreviewConfigVersion failed: %v", err)
	}
	if lookupCalls != 1 {
		t.Fatalf("expected one memoized DNS lookup across global and pool renders, got %d", lookupCalls)
	}
	firstServer := "server 93.184.216.34:8443 max_fails=3 fail_timeout=10s;"
	secondServer := "server 93.184.216.35:8443 max_fails=3 fail_timeout=10s;"
	firstIndex := strings.Index(preview.RouteConfig, firstServer)
	secondIndex := strings.Index(preview.RouteConfig, secondServer)
	if firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Fatalf("expected sorted resolved servers in preview route config, got %s", preview.RouteConfig)
	}
}

func TestRenderRouteConfigDisablesProxyBufferingForStreamingRoutes(t *testing.T) {
	routeConfig, _, err := renderRouteConfig(
		[]*model.ProxyRoute{
			{
				SiteName:                   "emby",
				Domain:                     "emby.example.com",
				Domains:                    mustJSON(t, []string{"emby.example.com"}),
				OriginURL:                  "https://origin.internal",
				Upstreams:                  mustJSON(t, []string{"https://origin.internal"}),
				Enabled:                    true,
				CustomHeaders:              "[]",
				CacheRules:                 "[]",
				ProxyBufferingMode:         proxyRouteProxyBufferingModeOff,
				RegionRestrictionMode:      proxyRouteRegionModeBlock,
				RegionRestrictionCountries: "[]",
				WAFMode:                    proxyRouteWAFModeBlock,
				CCMode:                     proxyRouteCCModeBlock,
			},
		},
		buildOpenRestyConfigSnapshot(),
	)
	if err != nil {
		t.Fatalf("renderRouteConfig failed: %v", err)
	}

	for _, expected := range []string{
		"proxy_buffering off;",
		"proxy_request_buffering off;",
		"proxy_max_temp_file_size 0;",
	} {
		if !strings.Contains(routeConfig, expected) {
			t.Fatalf("expected route config to contain %q, got %s", expected, routeConfig)
		}
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
