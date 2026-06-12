package service

import (
	"dushengcdn/common"
	"dushengcdn/internal/dnsworker"
	"dushengcdn/model"
	"strings"
	"time"
)

type AuthoritativeDNSSnapshotConditionalResult struct {
	NotModified     bool
	SnapshotVersion string
	Snapshot        *dnsworker.SignedSnapshot
}

func GetAuthoritativeDNSSnapshot(worker *model.DNSWorker) (*AuthoritativeDNSSnapshot, error) {
	return getAuthoritativeDNSSnapshotWithQueries(worker, gslbDNSSchedulingDataQueries{})
}

func GetSignedAuthoritativeDNSSnapshot(worker *model.DNSWorker, token string) (*dnsworker.SignedSnapshot, error) {
	return getSignedAuthoritativeDNSSnapshotWithQueries(worker, token, gslbDNSSchedulingDataQueries{})
}

func GetSignedAuthoritativeDNSSnapshotConditional(worker *model.DNSWorker, token string, lastSnapshotVersion string) (*AuthoritativeDNSSnapshotConditionalResult, error) {
	return getSignedAuthoritativeDNSSnapshotConditionalWithQueries(worker, token, lastSnapshotVersion, gslbDNSSchedulingDataQueries{})
}

func getSignedAuthoritativeDNSSnapshotConditionalWithQueries(worker *model.DNSWorker, token string, lastSnapshotVersion string, schedulingQueries gslbDNSSchedulingDataQueries) (*AuthoritativeDNSSnapshotConditionalResult, error) {
	lastSnapshotVersion = normalizeAuthoritativeDNSSnapshotVersionInput(lastSnapshotVersion)
	if lastSnapshotVersion == "" {
		snapshot, err := getSignedAuthoritativeDNSSnapshotWithQueries(worker, token, schedulingQueries)
		if err != nil {
			return nil, err
		}
		return &AuthoritativeDNSSnapshotConditionalResult{
			SnapshotVersion: signedAuthoritativeDNSSnapshotVersion(snapshot),
			Snapshot:        snapshot,
		}, nil
	}
	cacheKey, cacheable := authoritativeDNSSnapshotCacheKey(worker, schedulingQueries)
	if cacheable {
		if cached, ok := authoritativeDNSSnapshotCache.getSigned(cacheKey, token); ok {
			version := signedAuthoritativeDNSSnapshotVersion(cached)
			if version == lastSnapshotVersion {
				if worker != nil {
					_ = recordDNSWorkerSnapshotPull(worker, version)
				}
				return &AuthoritativeDNSSnapshotConditionalResult{
					NotModified:     true,
					SnapshotVersion: version,
				}, nil
			}
			if worker != nil {
				_ = recordDNSWorkerSnapshotPull(worker, version)
			}
			return &AuthoritativeDNSSnapshotConditionalResult{
				SnapshotVersion: version,
				Snapshot:        cached,
			}, nil
		}
		if cachedSnapshot, ok := authoritativeDNSSnapshotCache.getSnapshot(cacheKey); ok {
			version := normalizeAuthoritativeDNSSnapshotVersionInput(cachedSnapshot.SnapshotVersion)
			if version == lastSnapshotVersion {
				if worker != nil {
					_ = recordDNSWorkerSnapshotPull(worker, version)
				}
				return &AuthoritativeDNSSnapshotConditionalResult{
					NotModified:     true,
					SnapshotVersion: version,
				}, nil
			}
			if worker != nil {
				_ = recordDNSWorkerSnapshotPull(worker, version)
			}
			signed, err := dnsworker.SignSnapshot(convertAuthoritativeSnapshotToWorker(cachedSnapshot), token)
			if err != nil {
				return nil, err
			}
			authoritativeDNSSnapshotCache.storeSigned(cacheKey, token, signed)
			return &AuthoritativeDNSSnapshotConditionalResult{
				SnapshotVersion: version,
				Snapshot:        signed,
			}, nil
		}
	}
	if cacheable {
		snapshot, err := buildAuthoritativeDNSSnapshotWithQueries(worker, schedulingQueries)
		if err != nil {
			return nil, err
		}
		authoritativeDNSSnapshotCache.storeSnapshot(cacheKey, snapshot)
		signed, err := dnsworker.SignSnapshot(convertAuthoritativeSnapshotToWorker(snapshot), token)
		if err != nil {
			return nil, err
		}
		authoritativeDNSSnapshotCache.storeSigned(cacheKey, token, signed)
		return &AuthoritativeDNSSnapshotConditionalResult{
			SnapshotVersion: normalizeAuthoritativeDNSSnapshotVersionInput(snapshot.SnapshotVersion),
			Snapshot:        signed,
		}, nil
	}
	snapshot, err := getSignedAuthoritativeDNSSnapshotWithQueries(worker, token, schedulingQueries)
	if err != nil {
		return nil, err
	}
	return &AuthoritativeDNSSnapshotConditionalResult{
		SnapshotVersion: signedAuthoritativeDNSSnapshotVersion(snapshot),
		Snapshot:        snapshot,
	}, nil
}

func getSignedAuthoritativeDNSSnapshotWithQueries(worker *model.DNSWorker, token string, schedulingQueries gslbDNSSchedulingDataQueries) (*dnsworker.SignedSnapshot, error) {
	cacheKey, cacheable := authoritativeDNSSnapshotCacheKey(worker, schedulingQueries)
	if cacheable {
		if cached, ok := authoritativeDNSSnapshotCache.getSigned(cacheKey, token); ok {
			if worker != nil {
				_ = recordDNSWorkerSnapshotPull(worker, cached.Snapshot.SnapshotVersion)
			}
			return cached, nil
		}
		if cachedSnapshot, ok := authoritativeDNSSnapshotCache.getSnapshot(cacheKey); ok {
			if worker != nil {
				_ = recordDNSWorkerSnapshotPull(worker, cachedSnapshot.SnapshotVersion)
			}
			signed, err := dnsworker.SignSnapshot(convertAuthoritativeSnapshotToWorker(cachedSnapshot), token)
			if err != nil {
				return nil, err
			}
			authoritativeDNSSnapshotCache.storeSigned(cacheKey, token, signed)
			return signed, nil
		}
	}
	snapshot, err := buildAuthoritativeDNSSnapshotWithQueries(worker, schedulingQueries)
	if err != nil {
		return nil, err
	}
	if cacheable {
		authoritativeDNSSnapshotCache.storeSnapshot(cacheKey, snapshot)
	}
	signed, err := dnsworker.SignSnapshot(convertAuthoritativeSnapshotToWorker(snapshot), token)
	if err != nil {
		return nil, err
	}
	if cacheable {
		authoritativeDNSSnapshotCache.storeSigned(cacheKey, token, signed)
	}
	return signed, nil
}

func signedAuthoritativeDNSSnapshotVersion(snapshot *dnsworker.SignedSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return normalizeAuthoritativeDNSSnapshotVersionInput(snapshot.Snapshot.SnapshotVersion)
}

func normalizeAuthoritativeDNSSnapshotVersionInput(version string) string {
	version = strings.TrimSpace(version)
	version = strings.Trim(version, `"`)
	return strings.TrimSpace(version)
}

func authoritativeDNSWorkerPolicy() dnsworker.WorkerPolicy {
	return dnsworker.WorkerPolicy{
		QueryRateLimit:    nonNegativeInt(common.AuthoritativeDNSWorkerQueryRateLimit),
		ResponseRateLimit: nonNegativeInt(common.AuthoritativeDNSWorkerResponseRateLimit),
		UDPResponseSize:   clampInt(common.AuthoritativeDNSWorkerUDPResponseSize, 512, 65535),
		ECSEnabled:        common.AuthoritativeDNSWorkerECSEnabled,
		ECSIPv4Prefix:     clampInt(common.AuthoritativeDNSWorkerECSIPv4Prefix, 0, 32),
		ECSIPv6Prefix:     clampInt(common.AuthoritativeDNSWorkerECSIPv6Prefix, 0, 128),
	}
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func getAuthoritativeDNSSnapshotWithQueries(worker *model.DNSWorker, schedulingQueries gslbDNSSchedulingDataQueries) (*AuthoritativeDNSSnapshot, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	cacheKey, cacheable := authoritativeDNSSnapshotCacheKey(worker, schedulingQueries)
	if cacheable {
		if cached, ok := authoritativeDNSSnapshotCache.getSnapshot(cacheKey); ok {
			if worker != nil {
				_ = recordDNSWorkerSnapshotPull(worker, cached.SnapshotVersion)
			}
			return cached, nil
		}
	}
	snapshot, err := buildAuthoritativeDNSSnapshotWithQueries(worker, schedulingQueries)
	if err != nil {
		return nil, err
	}
	if cacheable {
		authoritativeDNSSnapshotCache.storeSnapshot(cacheKey, snapshot)
	}
	return snapshot, nil
}

func recordDNSWorkerSnapshotPull(worker *model.DNSWorker, version string) error {
	if worker == nil {
		return nil
	}
	now := time.Now()
	worker.Status = dnsWorkerStatusOnline
	worker.LastSeenAt = &now
	worker.LastSnapshotAt = &now
	worker.LastSnapshotVersion = strings.TrimSpace(version)
	return worker.UpdateSnapshotPull()
}
