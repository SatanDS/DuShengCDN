package job

import (
	"dushengcdn/service"
	"log/slog"
)

type CloudflareDNSReconcileJob struct{}

func (j *CloudflareDNSReconcileJob) Run() {
	if err := service.ReconcileCloudflareDNSAutomation(); err != nil {
		slog.Warn("cloudflare dns reconcile job failed", "error", err)
	}
}
