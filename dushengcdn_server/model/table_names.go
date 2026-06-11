package model

func (AcmeAccount) TableName() string {
	return "acme_accounts"
}

func (ApplyLog) TableName() string {
	return "apply_logs"
}

func (AuthSource) TableName() string {
	return "auth_sources"
}

func (ConfigVersion) TableName() string {
	return "config_versions"
}

func (ConfigVersionArtifact) TableName() string {
	return "config_version_artifacts"
}

func (DNSQueryRollup) TableName() string {
	return "dns_query_rollups"
}

func (DNSRecord) TableName() string {
	return "dns_records"
}

func (DNSWorker) TableName() string {
	return "dns_workers"
}

func (DNSWorkerNodeProbe) TableName() string {
	return "dns_worker_node_probes"
}

func (DNSSECKey) TableName() string {
	return "dnssec_keys"
}

func (DNSZoneWorkerAssignment) TableName() string {
	return "dns_zone_worker_assignments"
}

func (DNSZone) TableName() string {
	return "dns_zones"
}

func (DnsAccount) TableName() string {
	return "dns_accounts"
}

func (ExternalAccount) TableName() string {
	return "external_accounts"
}

func (File) TableName() string {
	return "files"
}

func (GSLBSchedulingState) TableName() string {
	return "gslb_scheduling_states"
}

func (ManagedDomain) TableName() string {
	return "managed_domains"
}

func (Node) TableName() string {
	return "nodes"
}

func (NodeAccessLog) TableName() string {
	return "node_access_logs"
}

func (NodeHealthEvent) TableName() string {
	return "node_health_events"
}

func (NodeMetricSnapshot) TableName() string {
	return "node_metric_snapshots"
}

func (NodeRequestReport) TableName() string {
	return "node_request_reports"
}

func (NodeSystemProfile) TableName() string {
	return "node_system_profiles"
}

func (OriginHealthStatus) TableName() string {
	return "origin_health_statuses"
}

func (Option) TableName() string {
	return "options"
}

func (Origin) TableName() string {
	return "origins"
}

func (ProxyRoute) TableName() string {
	return "proxy_routes"
}

func (TLSCertificate) TableName() string {
	return "tls_certificates"
}

func (User) TableName() string {
	return "users"
}
