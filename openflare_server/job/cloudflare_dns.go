package job

import (
	"log/slog"
	"openflare/service"
)

type CloudflareDNSReconcileJob struct{}

func (j *CloudflareDNSReconcileJob) Run() {
	if err := service.ReconcileCloudflareDNSAutomation(); err != nil {
		slog.Warn("cloudflare dns reconcile job failed", "error", err)
	}
}
