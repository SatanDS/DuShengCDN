package router

import (
	"dushengcdn/controller"
	"dushengcdn/middleware"

	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	workerAPIRateLimit := middleware.DNSWorkerAPIRateLimit()
	apiRouter.GET("/dns-snapshot", workerAPIRateLimit, controller.GetDNSSnapshot)
	apiRouter.POST("/dns-worker-heartbeat", workerAPIRateLimit, controller.DNSWorkerHeartbeat)
	apiRouter.GET("/dns-source-databases/manifest", workerAPIRateLimit, controller.GetDNSSourceDatabaseManifest)
	apiRouter.GET("/dns-source-databases/files/:kind/:name", workerAPIRateLimit, controller.DownloadDNSSourceDatabaseFile)

	apiRouter.Use(middleware.GlobalAPIRateLimit())
	{
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/about", controller.GetAbout)
		apiRouter.POST("/verification", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.POST("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), controller.ResetPassword)
		apiRouter.GET("/oauth/github/authorize", middleware.CriticalRateLimit(), controller.GitHubOAuthAuthorize)
		apiRouter.GET("/oauth/github", middleware.CriticalRateLimit(), controller.GitHubOAuth)
		apiRouter.GET("/oauth/wechat/authorize", middleware.CriticalRateLimit(), controller.WeChatOAuthAuthorize)
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.POST("/oauth/wechat/bind", middleware.CriticalRateLimit(), middleware.UserAuth(), middleware.NoTokenAuth(), controller.WeChatBindPost)
		apiRouter.POST("/oauth/email/verification", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), middleware.UserAuth(), middleware.NoTokenAuth(), controller.SendEmailBindVerification)
		apiRouter.POST("/oauth/email/bind", middleware.CriticalRateLimit(), middleware.UserAuth(), middleware.NoTokenAuth(), controller.EmailBindPost)
		apiRouter.GET("/oauth/:source/authorize", middleware.CriticalRateLimit(), controller.OAuthAuthorize)
		apiRouter.GET("/oauth/:source/callback", middleware.CriticalRateLimit(), controller.OAuthCallback)
		apiRouter.POST("/oauth/link-existing", middleware.CriticalRateLimit(), controller.LinkExistingOAuthAccount)
		externalAccountRoute := apiRouter.Group("/oauth/external-accounts")
		externalAccountRoute.Use(middleware.UserAuth(), middleware.NoTokenAuth())
		{
			externalAccountRoute.GET("/", controller.ListExternalAccounts)
			externalAccountRoute.POST("/:id/delete", controller.DeleteExternalAccount)
		}

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), controller.Login)
			userRoute.POST("/logout", middleware.UserAuth(), middleware.NoTokenAuth(), controller.Logout)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth(), middleware.NoTokenAuth())
			{
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.POST("/self/update", controller.UpdateSelf)
				selfRoute.POST("/self/delete", controller.DeleteSelf)
				selfRoute.POST("/token", controller.GenerateToken)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.POST("/update", controller.UpdateUser)
				adminRoute.POST("/:id/delete", controller.DeleteUser)
			}
		}
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth(), middleware.NoTokenAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.POST("/update", controller.UpdateOption)
			optionRoute.POST("/update-batch", controller.UpdateOptionsBatch)
			optionRoute.POST("/geoip/lookup", controller.LookupGeoIP)
			optionRoute.POST("/database/cleanup", controller.CleanupDatabaseObservability)
		}
		authSourceRoute := apiRouter.Group("/auth-sources")
		authSourceRoute.Use(middleware.RootAuth(), middleware.NoTokenAuth())
		{
			authSourceRoute.GET("/", controller.ListAuthSources)
			authSourceRoute.POST("/", controller.CreateAuthSource)
			authSourceRoute.POST("/:id/update", controller.UpdateAuthSource)
			authSourceRoute.POST("/:id/delete", controller.DeleteAuthSource)
			authSourceRoute.POST("/:id/toggle", controller.ToggleAuthSource)
		}
		updateRoute := apiRouter.Group("/update")
		updateRoute.Use(middleware.RootAuth(), middleware.NoTokenAuth())
		{
			updateRoute.GET("/latest-release", controller.GetLatestRelease)
			updateRoute.GET("/logs/ws", controller.StreamServerUpgradeLogs)
			updateRoute.POST("/manual-upload", controller.UploadManualServerBinary)
			updateRoute.POST("/manual-upgrade", controller.ConfirmManualServerUpgrade)
			updateRoute.POST("/upgrade", controller.UpgradeServer)
		}
		licenseActivationRoute := apiRouter.Group("/license/activation")
		licenseActivationRoute.Use(middleware.CriticalRateLimit())
		{
			licenseActivationRoute.POST("/activate", controller.ActivateCommercialLicenseLease)
			licenseActivationRoute.POST("/renew", controller.RenewCommercialLicenseActivationLease)
		}
		licenseRoute := apiRouter.Group("/license")
		licenseRoute.Use(middleware.RootAuth(), middleware.NoTokenAuth())
		{
			licenseRoute.GET("/status", controller.GetCommercialLicense)
			licenseRoute.POST("/install", controller.InstallCommercialLicense)
			licenseRoute.POST("/activate", controller.ActivateCommercialLicense)
			licenseRoute.POST("/renew", controller.RenewCommercialLicenseLease)
			licenseRoute.POST("/clear", controller.ClearCommercialLicense)
			licenseRoute.GET("/issuer", controller.GetCommercialLicenseIssuer)
			licenseRoute.POST("/issue", controller.IssueCommercialLicense)
			licenseRoute.GET("/activations", controller.ListCommercialLicenseActivations)
			licenseRoute.POST("/revoke", controller.RevokeCommercialLicense)
			licenseRoute.POST("/restore", controller.RestoreCommercialLicense)
			licenseRoute.POST("/delete", controller.DeleteCommercialLicenseActivation)
		}
		proxyRoute := apiRouter.Group("/proxy-routes")
		proxyRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			proxyRoute.GET("/", controller.GetProxyRoutes)
			proxyRoute.GET("/:id", controller.GetProxyRoute)
			proxyRoute.POST("/", controller.CreateProxyRoute)
			proxyRoute.POST("/:id/update", controller.UpdateProxyRoute)
			proxyRoute.POST("/:id/dns/switch-authoritative", controller.SwitchProxyRouteToAuthoritativeDNS)
			proxyRoute.POST("/:id/delete", controller.DeleteProxyRoute)
			proxyRoute.POST("/:id/cache/purge", controller.PurgeProxyRouteCache)
			proxyRoute.POST("/:id/cache/warm", controller.WarmProxyRouteCache)
		}
		configReleasePlanRoute := apiRouter.Group("/config-release-plans")
		configReleasePlanRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			configReleasePlanRoute.GET("/", controller.ListConfigReleasePlans)
			configReleasePlanRoute.GET("/:id", controller.GetConfigReleasePlan)
			configReleasePlanRoute.POST("/", controller.CreateConfigReleasePlan)
			configReleasePlanRoute.POST("/:id/start", controller.StartConfigReleasePlan)
			configReleasePlanRoute.POST("/:id/evaluate", controller.EvaluateConfigReleasePlan)
			configReleasePlanRoute.POST("/:id/advance", controller.AdvanceConfigReleasePlan)
			configReleasePlanRoute.POST("/:id/complete", controller.CompleteConfigReleasePlan)
			configReleasePlanRoute.POST("/:id/fail", controller.FailConfigReleasePlan)
		}
		originRoute := apiRouter.Group("/origins")
		originRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			originRoute.GET("/", controller.GetOrigins)
			originRoute.GET("/:id", controller.GetOrigin)
			originRoute.POST("/", controller.CreateOrigin)
			originRoute.POST("/:id/update", controller.UpdateOrigin)
			originRoute.POST("/:id/delete", controller.DeleteOrigin)
		}
		managedDomainRoute := apiRouter.Group("/managed-domains")
		managedDomainRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			managedDomainRoute.GET("/", controller.GetManagedDomains)
			managedDomainRoute.GET("/match", controller.MatchManagedDomainCertificate)
			managedDomainRoute.POST("/", controller.CreateManagedDomain)
			managedDomainRoute.POST("/:id/update", controller.UpdateManagedDomain)
			managedDomainRoute.POST("/:id/delete", controller.DeleteManagedDomain)
		}
		tlsCertificateRoute := apiRouter.Group("/tls-certificates")
		tlsCertificateRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			tlsCertificateRoute.GET("/", controller.GetTLSCertificates)
			tlsCertificateRoute.GET("/:id", controller.GetTLSCertificate)
			tlsCertificateRoute.POST("/:id/content", controller.GetTLSCertificateContent)
			tlsCertificateRoute.POST("/", controller.CreateTLSCertificate)
			tlsCertificateRoute.POST("/:id/update", controller.UpdateTLSCertificate)
			tlsCertificateRoute.POST("/:id/update-acme", controller.UpdateAcmeCertificate)
			tlsCertificateRoute.POST("/:id/convert-acme", controller.ConvertTLSCertificateToAcme)
			tlsCertificateRoute.POST("/import-file", controller.ImportTLSCertificateFile)
			tlsCertificateRoute.POST("/:id/delete", controller.DeleteTLSCertificate)
			tlsCertificateRoute.POST("/apply", controller.ApplyTLSCertificate)
			tlsCertificateRoute.POST("/:id/renew", controller.RenewTLSCertificate)
		}
		acmeAccountRoute := apiRouter.Group("/acme-accounts")
		acmeAccountRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			acmeAccountRoute.GET("/default", controller.GetDefaultAcmeAccount)
		}
		dnsAccountRoute := apiRouter.Group("/dns-accounts")
		dnsAccountRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			dnsAccountRoute.GET("/", controller.GetDnsAccounts)
			dnsAccountRoute.POST("/", controller.CreateDnsAccount)
			dnsAccountRoute.POST("/:id/update", controller.UpdateDnsAccount)
			dnsAccountRoute.POST("/:id/delete", controller.DeleteDnsAccount)
		}
		dnsZoneRoute := apiRouter.Group("/dns-zones")
		dnsZoneRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			dnsZoneRoute.GET("/", controller.GetDNSZones)
			dnsZoneRoute.GET("/:id", controller.GetDNSZone)
			dnsZoneRoute.POST("/", controller.CreateDNSZone)
			dnsZoneRoute.POST("/:id/update", controller.UpdateDNSZone)
			dnsZoneRoute.POST("/:id/delete", controller.DeleteDNSZone)
			dnsZoneRoute.GET("/:id/workers", controller.GetDNSZoneWorkers)
			dnsZoneRoute.POST("/:id/workers", controller.UpdateDNSZoneWorkers)
			dnsZoneRoute.GET("/:id/dnssec", controller.GetDNSZoneDNSSEC)
			dnsZoneRoute.POST("/:id/dnssec/enable", controller.EnableDNSZoneDNSSEC)
			dnsZoneRoute.POST("/:id/dnssec/disable", controller.DisableDNSZoneDNSSEC)
			dnsZoneRoute.POST("/:id/dnssec/rotate-zsk", controller.RotateDNSZoneDNSSECZSK)
			dnsZoneRoute.POST("/:id/dnssec/rotate-ksk", controller.RotateDNSZoneDNSSECKSK)
			dnsZoneRoute.GET("/:id/dnssec/ds", controller.GetDNSZoneDNSSECDS)
			dnsZoneRoute.GET("/:id/delegation-check", controller.CheckDNSZoneDelegation)
			dnsZoneRoute.GET("/:id/records", controller.GetDNSZoneRecords)
			dnsZoneRoute.POST("/:id/records", controller.CreateDNSZoneRecord)
		}
		dnsRecordRoute := apiRouter.Group("/dns-records")
		dnsRecordRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			dnsRecordRoute.POST("/:id/update", controller.UpdateDNSRecord)
			dnsRecordRoute.POST("/:id/delete", controller.DeleteDNSRecord)
		}
		dnsWorkerAdminRoute := apiRouter.Group("/dns-workers")
		dnsWorkerAdminRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			dnsWorkerAdminRoute.GET("/", controller.GetDNSWorkers)
			dnsWorkerAdminRoute.GET("/observability", controller.GetDNSObservability)
			dnsWorkerAdminRoute.GET("/migration-candidates", controller.GetDNSMigrationCandidates)
			dnsWorkerAdminRoute.GET("/scheduling-states", controller.GetDNSGSLBSchedulingStates)
			dnsWorkerAdminRoute.POST("/simulate", controller.SimulateDNSGSLB)
			dnsWorkerAdminRoute.POST("/", controller.CreateDNSWorker)
			dnsWorkerAdminRoute.POST("/:id/update-info", controller.UpdateDNSWorker)
			dnsWorkerAdminRoute.POST("/:id/rotate-token", controller.RotateDNSWorkerToken)
			dnsWorkerAdminRoute.POST("/:id/revoke-token", controller.RevokeDNSWorkerToken)
			dnsWorkerAdminRoute.POST("/:id/probe", controller.ProbeDNSWorker)
			dnsWorkerAdminRoute.POST("/:id/update", controller.RequestDNSWorkerUpdate)
			dnsWorkerAdminRoute.POST("/:id/delete", controller.DeleteDNSWorker)
		}
		dnsSourceDatabaseAdminRoute := apiRouter.Group("/dns-source-databases")
		dnsSourceDatabaseAdminRoute.Use(middleware.RootAuth(), middleware.NoTokenAuth())
		{
			dnsSourceDatabaseAdminRoute.GET("/status", controller.GetDNSSourceDatabaseMirrorStatus)
			dnsSourceDatabaseAdminRoute.POST("/refresh", controller.RefreshDNSSourceDatabaseMirror)
		}
		configVersionRoute := apiRouter.Group("/config-versions")
		configVersionRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			configVersionRoute.GET("/", controller.GetConfigVersions)
			configVersionRoute.GET("/active", controller.GetActiveConfigVersion)
			configVersionRoute.GET("/preview", controller.PreviewConfigVersion)
			configVersionRoute.GET("/diff", controller.DiffConfigVersion)
			configVersionRoute.GET("/:id", controller.GetConfigVersion)
			configVersionRoute.POST("/publish", controller.PublishConfigVersion)
			configVersionRoute.POST("/:id/activate", controller.ActivateConfigVersion)
			configVersionRoute.POST("/cleanup", controller.CleanupConfigVersions)
		}
		dashboardRoute := apiRouter.Group("/dashboard")
		dashboardRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			dashboardRoute.GET("/overview", controller.GetDashboardOverview)
		}
		nodeRoute := apiRouter.Group("/nodes")
		nodeRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			nodeRoute.GET("/bootstrap-token", controller.GetNodeBootstrapToken)
			nodeRoute.POST("/bootstrap-token/rotate", controller.RotateNodeBootstrapToken)
			nodeRoute.GET("/", controller.GetNodes)
			nodeRoute.POST("/", controller.CreateNode)
			nodeRoute.GET("/:id/agent-release", controller.GetNodeAgentRelease)
			nodeRoute.POST("/:id/update", controller.UpdateNode)
			nodeRoute.POST("/:id/delete", controller.DeleteNode)
			nodeRoute.POST("/:id/agent-token/rotate", controller.RotateNodeAgentToken)
			nodeRoute.POST("/:id/agent-update", controller.RequestNodeAgentUpdate)
			nodeRoute.POST("/:id/openresty-restart", controller.RequestNodeOpenrestyRestart)
			nodeRoute.POST("/:id/force-sync", controller.RequestNodeForceSync)
			nodeRoute.GET("/:id/observability", controller.GetNodeObservability)
			nodeRoute.POST("/:id/observability/cleanup", controller.CleanupNodeHealthEvents)
		}
		applyLogRoute := apiRouter.Group("/apply-logs")
		applyLogRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			applyLogRoute.GET("/", controller.GetApplyLogs)
			applyLogRoute.POST("/cleanup", controller.CleanupApplyLogs)
		}
		accessLogRoute := apiRouter.Group("/access-logs")
		accessLogRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())
		{
			accessLogRoute.GET("/", controller.GetAccessLogs)
			accessLogRoute.GET("/folds", controller.GetFoldedAccessLogs)
			accessLogRoute.GET("/ip-summary", controller.GetAccessLogIPSummaries)
			accessLogRoute.GET("/ip-summary/trend", controller.GetAccessLogIPTrend)
			accessLogRoute.GET("/metering-overview", controller.GetObservabilityMeteringOverview)
			accessLogRoute.POST("/cleanup", controller.CleanupAccessLogs)
		}
		agentRoute := apiRouter.Group("/agent")
		{
			discoveryRoute := agentRoute.Group("/")
			discoveryRoute.Use(middleware.AgentRegisterAuth())
			{
				discoveryRoute.POST("/nodes/register", controller.AgentRegister)
			}
			authorizedRoute := agentRoute.Group("/")
			authorizedRoute.Use(middleware.AgentAuth())
			{
				authorizedRoute.GET("/ws", controller.AgentWebSocket)
				authorizedRoute.POST("/nodes/heartbeat", controller.AgentHeartbeat)
				authorizedRoute.GET("/config-versions/active", controller.AgentGetActiveConfig)
				authorizedRoute.POST("/apply-logs", controller.AgentReportApplyLog)
			}
		}
	}
}
