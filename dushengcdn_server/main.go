package main

import (
	"context"
	"dushengcdn/common"
	_ "dushengcdn/docs"
	"dushengcdn/job"
	"dushengcdn/middleware"
	"dushengcdn/model"
	"dushengcdn/router"
	"dushengcdn/service"
	"dushengcdn/utils/geoip"
	"dushengcdn/utils/security"
	"embed"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

//go:embed all:web/build
var buildFS embed.FS

//go:embed web/build/index.html
var indexPage []byte

// @title DuShengCDN Server API
// @version 3.0
// @description DuShengCDN Server 管理端与 Agent API 文档。
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 仅部分兼容接口支持长期 Bearer 用户 Token；高危管理接口要求浏览器 session cookie 和 CSRF Token
// @securityDefinitions.apikey AgentTokenAuth
// @in header
// @name X-Agent-Token
// @description Agent API 使用节点专属 Agent Token 或全局 Discovery Token
func main() {
	shouldExit, err := common.InitServerRuntime()
	if err != nil {
		slog.Error("initialize runtime config failed", "error", err)
		os.Exit(1)
	}
	if shouldExit {
		return
	}

	common.SetupGinLog()
	slog.Info("DuShengCDN started", "version", common.Version)
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	// Initialize SQL Database
	err = model.InitDB()
	if err != nil {
		slog.Error("initialize database failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		err := model.CloseDB()
		if err != nil {
			slog.Error("close database failed", "error", err)
			os.Exit(1)
		}
	}()

	if rootPasswordResetRequested() {
		password, err := readRootPasswordResetPassword()
		if err != nil {
			slog.Error("read root password reset input failed", "error", err)
			os.Exit(1)
		}
		if err := model.ResetRootPassword(password); err != nil {
			slog.Error("reset root password failed", "error", err)
			os.Exit(1)
		}
		slog.Info("root password reset completed")
		return
	}

	if *common.CreateDNSWorkerName != "" {
		worker, err := service.CreateAuthoritativeDNSWorker(service.DNSWorkerInput{
			Name:          *common.CreateDNSWorkerName,
			PublicAddress: *common.CreateDNSWorkerPublicAddress,
		})
		if err != nil {
			slog.Error("create DNS worker failed", "error", err)
			os.Exit(1)
		}
		fmt.Println(worker.Token)
		return
	}

	// Initialize Redis
	err = common.InitRedisClient()
	if err != nil {
		slog.Error("initialize redis failed", "error", err)
		os.Exit(1)
	}

	// Initialize options
	model.InitOptionMap()
	geoip.InitGeoIP()
	backgroundCtx, cancelBackgroundTasks := context.WithCancel(context.Background())
	defer cancelBackgroundTasks()
	service.StartDatabaseAutoCleanupScheduler(backgroundCtx)
	service.StartCommercialLicenseLeaseRenewer(backgroundCtx)

	job.InitCronJobs()
	defer job.StopCronJobs()
	job.WarmupDNSSourceDatabaseMirror()

	if err := validateRuntimeSecurityConfig(gin.Mode()); err != nil {
		slog.Error("runtime security validation failed", "error", err)
		os.Exit(1)
	}

	// Initialize HTTP server
	server := gin.New()
	server.Use(ginSanitizedLogger(), gin.Recovery())
	configureTrustedProxies(server)
	//server.Use(gzip.Gzip(gzip.DefaultCompression))
	server.Use(middleware.CORS())
	server.Use(middleware.JSONBodyLimit())

	// Initialize session store
	sessionStoreConfigured := false
	if common.RedisEnabled {
		opt, err := common.ParseRedisOption()
		if err != nil {
			if common.RedisRequired {
				slog.Error("parse redis session options failed", "error", err)
				os.Exit(1)
			}
			common.DisableRedisClient()
			slog.Warn("falling back to cookie session because redis session options failed", "error", err)
		} else {
			var store sessions.Store
			if opt.DB > 0 {
				store, err = redis.NewStoreWithDB(opt.MinIdleConns, opt.Network, opt.Addr, opt.Password, strconv.Itoa(opt.DB), []byte(common.SessionSecret))
			} else {
				store, err = redis.NewStore(opt.MinIdleConns, opt.Network, opt.Addr, opt.Password, []byte(common.SessionSecret))
			}
			if err != nil {
				if common.RedisRequired {
					slog.Error("initialize redis session store failed", "error", err)
					os.Exit(1)
				}
				common.DisableRedisClient()
				slog.Warn("falling back to cookie session because redis session store failed", "error", err)
			} else {
				configureSessionStore(store)
				server.Use(sessions.Sessions("session", store))
				sessionStoreConfigured = true
			}
		}
	}
	if !sessionStoreConfigured {
		store := cookie.NewStore([]byte(common.SessionSecret))
		configureSessionStore(store)
		server.Use(sessions.Sessions("session", store))
	}

	router.SetRouter(server, buildFS, indexPage)
	var port = os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}
	dbBackend := "sqlite"
	if common.SQLDSN != "" {
		dbBackend = "postgres"
	}
	listenAddress := net.JoinHostPort(strings.TrimSpace(common.ListenAddress), port)
	slog.Info("server config", "listen_address", common.ListenAddress, "port", port, "gin_mode", gin.Mode(), "log_level", common.GetLogLevel(), "db_backend", dbBackend, "sqlite_path", common.SQLitePath, "redis_enabled", common.RedisEnabled, "upload_path", common.UploadPath, "log_dir", valueOrDefault(*common.LogDir, "stdout"), "agent_token_configured", common.AgentToken != "", "node_offline_threshold", common.NodeOfflineThreshold)
	slog.Info("server listening", "address", listenAddress)
	err = server.Run(listenAddress)
	if err != nil {
		slog.Error("server run failed", "error", err)
	}
}

func rootPasswordResetRequested() bool {
	return *common.ResetRootPassword != "" ||
		strings.TrimSpace(*common.ResetRootPasswordFile) != "" ||
		*common.ResetRootPasswordStdin
}

func readRootPasswordResetPassword() (string, error) {
	sources := 0
	if *common.ResetRootPassword != "" {
		sources++
	}
	if strings.TrimSpace(*common.ResetRootPasswordFile) != "" {
		sources++
	}
	if *common.ResetRootPasswordStdin {
		sources++
	}
	if sources != 1 {
		return "", fmt.Errorf("exactly one root password reset input source is required")
	}

	var password string
	switch {
	case strings.TrimSpace(*common.ResetRootPasswordFile) != "":
		raw, err := os.ReadFile(strings.TrimSpace(*common.ResetRootPasswordFile))
		if err != nil {
			return "", err
		}
		password = strings.TrimRight(string(raw), "\r\n")
	case *common.ResetRootPasswordStdin:
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		password = strings.TrimRight(string(raw), "\r\n")
	default:
		slog.Warn("reset-root-password was supplied on argv; prefer --reset-root-password-file or --reset-root-password-stdin to avoid shell history and process-list exposure")
		password = *common.ResetRootPassword
	}
	if password == "" {
		return "", fmt.Errorf("root password reset input is empty")
	}
	return password, nil
}

func valueOrDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func configureSessionStore(store sessions.Store) {
	if store == nil {
		return
	}
	sameSite := http.SameSiteLaxMode
	switch strings.ToLower(strings.TrimSpace(common.SessionCookieSameSite)) {
	case "strict":
		sameSite = http.SameSiteStrictMode
	case "none":
		sameSite = http.SameSiteNoneMode
	case "default":
		sameSite = http.SameSiteDefaultMode
	case "lax", "":
		sameSite = http.SameSiteLaxMode
	}
	secure := strings.HasPrefix(strings.ToLower(strings.TrimSpace(common.ServerAddress)), "https://")
	if common.SessionCookieSecureConfigured {
		secure = common.SessionCookieSecure
	}
	store.Options(sessions.Options{
		Path:     "/",
		HttpOnly: true,
		SameSite: sameSite,
		Secure:   secure,
	})
}

func configureTrustedProxies(server *gin.Engine) {
	if server == nil {
		return
	}
	trustedProxies, err := parseTrustedProxies(common.TrustedProxies)
	if err != nil {
		slog.Error("configure trusted proxies failed", "error", err)
		os.Exit(1)
	}
	if err := server.SetTrustedProxies(trustedProxies); err != nil {
		slog.Error("configure trusted proxies failed", "error", err)
		os.Exit(1)
	}
}

func parseTrustedProxies(raw string) ([]string, error) {
	proxies := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	if len(proxies) == 0 {
		return nil, nil
	}
	for _, proxy := range proxies {
		if isGlobalTrustedProxy(proxy) {
			return nil, fmt.Errorf("TRUSTED_PROXIES must not trust all client networks: %s", proxy)
		}
	}
	return proxies, nil
}

func isGlobalTrustedProxy(proxy string) bool {
	if !strings.Contains(proxy, "/") {
		return false
	}
	_, ipNet, err := net.ParseCIDR(proxy)
	if err != nil {
		return false
	}
	ones, bits := ipNet.Mask.Size()
	return ones == 0 && (bits == net.IPv4len*8 || bits == net.IPv6len*8)
}

func ginSanitizedLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		path := ""
		if param.Request != nil && param.Request.URL != nil {
			path = param.Request.URL.EscapedPath()
			if path == "" {
				path = "/"
			}
		}
		errorMessage := security.RedactSensitiveText(param.ErrorMessage)
		if errorMessage != "" {
			errorMessage = " " + errorMessage
		}
		return fmt.Sprintf("[GIN] %v | %3d | %13v | %15s | %-7s %#v%s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			path,
			errorMessage,
		)
	})
}

func validateRuntimeSecurityConfig(ginMode string) error {
	if ginMode == gin.DebugMode {
		return nil
	}
	if strings.TrimSpace(os.Getenv("SESSION_SECRET")) == "" {
		return fmt.Errorf("SESSION_SECRET must be explicitly set in release mode")
	}
	secret := strings.TrimSpace(common.SessionSecret)
	if len(secret) < 32 {
		return fmt.Errorf("SESSION_SECRET must be at least 32 characters in release mode")
	}
	switch strings.ToLower(secret) {
	case "replace-with-random-string",
		"replace-with-a-long-random-string",
		"dev-session-secret",
		"test-session-secret":
		return fmt.Errorf("SESSION_SECRET is a placeholder and must be replaced before production startup")
	default:
		return nil
	}
}
