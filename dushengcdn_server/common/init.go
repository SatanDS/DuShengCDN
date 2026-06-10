package common

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	Port                         = flag.Int("port", 3000, "the listening port")
	PrintVersion                 = flag.Bool("version", false, "print version and exit")
	PrintHelp                    = flag.Bool("help", false, "print help and exit")
	LogDir                       = flag.String("log-dir", "", "specify the log directory")
	ResetRootPassword            = flag.String("reset-root-password", "", "reset root password and exit without starting the HTTP server; prefer --reset-root-password-file or --reset-root-password-stdin to avoid argv exposure")
	ResetRootPasswordFile        = flag.String("reset-root-password-file", "", "read the new root password from FILE, reset root password, and exit without starting the HTTP server")
	ResetRootPasswordStdin       = flag.Bool("reset-root-password-stdin", false, "read the new root password from stdin, reset root password, and exit without starting the HTTP server")
	CreateDNSWorkerName          = flag.String("create-dns-worker-name", "", "create a DNS Worker with this name, print its token, and exit without starting the HTTP server")
	CreateDNSWorkerPublicAddress = flag.String("create-dns-worker-public-address", "", "public address saved for --create-dns-worker-name")
)

// UploadPath Maybe override by ENV_VAR
var UploadPath = "upload"

func printHelp() {
	fmt.Println("DuShengCDN " + Version + " - Internal OpenResty Control Plane.")
	fmt.Println("Copyright (C) 2023 JustSong. All rights reserved.")
	fmt.Println("GitHub: https://github.com/SatanDS/DuShengCDN")
	fmt.Println("Usage: dushengcdn [--port <port>] [--log-dir <log directory>] [--reset-root-password-file <file>|--reset-root-password-stdin] [--create-dns-worker-name <name>] [--create-dns-worker-public-address <address>] [--version] [--help]")
}

func init() {
	executableName := strings.ToLower(filepath.Base(os.Args[0]))
	if !strings.Contains(executableName, ".test") {
		flag.Parse()
	}

	if *PrintVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	if *PrintHelp {
		printHelp()
		os.Exit(0)
	}

	if os.Getenv("SESSION_SECRET") != "" {
		SessionSecret = os.Getenv("SESSION_SECRET")
	}
	if os.Getenv("SESSION_COOKIE_SECURE") != "" {
		SessionCookieSecureConfigured = true
		SessionCookieSecure = readBoolEnv("SESSION_COOKIE_SECURE")
	}
	if strings.TrimSpace(os.Getenv("SESSION_COOKIE_SAME_SITE")) != "" {
		SessionCookieSameSite = strings.ToLower(strings.TrimSpace(os.Getenv("SESSION_COOKIE_SAME_SITE")))
	}
	if os.Getenv("DUSHENGCDN_INITIAL_ROOT_PASSWORD") != "" {
		InitialRootPassword = os.Getenv("DUSHENGCDN_INITIAL_ROOT_PASSWORD")
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_INITIAL_ROOT_PASSWORD_FILE")) != "" {
		InitialRootPasswordFile = strings.TrimSpace(os.Getenv("DUSHENGCDN_INITIAL_ROOT_PASSWORD_FILE"))
	}
	if os.Getenv("TRUSTED_PROXIES") != "" {
		TrustedProxies = os.Getenv("TRUSTED_PROXIES")
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_LISTEN_ADDRESS")) != "" {
		ListenAddress = strings.TrimSpace(os.Getenv("DUSHENGCDN_LISTEN_ADDRESS"))
	} else if strings.TrimSpace(os.Getenv("LISTEN_ADDRESS")) != "" {
		ListenAddress = strings.TrimSpace(os.Getenv("LISTEN_ADDRESS"))
	}
	if value := readPositiveInt64Env("DUSHENGCDN_JSON_BODY_MAX_BYTES"); value > 0 {
		JSONBodyMaxBytes = value
	} else if value := readPositiveInt64Env("JSON_BODY_MAX_BYTES"); value > 0 {
		JSONBodyMaxBytes = value
	}
	if os.Getenv("DUSHENGCDN_PUBLIC_STATUS_RUNTIME_METADATA") != "" {
		PublicStatusRuntimeMetadataEnabled = readBoolEnv("DUSHENGCDN_PUBLIC_STATUS_RUNTIME_METADATA")
	}
	if os.Getenv("SQLITE_PATH") != "" {
		SQLitePath = os.Getenv("SQLITE_PATH")
	}
	if os.Getenv("SQL_DSN") != "" {
		SQLDSN = os.Getenv("SQL_DSN")
	}
	if os.Getenv("DSN") != "" {
		SQLDSN = os.Getenv("DSN")
	}
	if value := readPositiveIntEnv("DATABASE_MAX_OPEN_CONNS"); value > 0 {
		DatabaseMaxOpenConns = value
	}
	if value := readPositiveIntEnv("DATABASE_MAX_IDLE_CONNS"); value > 0 {
		DatabaseMaxIdleConns = value
	}
	if value := readPositiveIntEnv("DATABASE_CONN_MAX_LIFETIME_SECONDS"); value > 0 {
		DatabaseConnMaxLifetime = time.Duration(value) * time.Second
	}
	if os.Getenv("UPLOAD_PATH") != "" {
		UploadPath = os.Getenv("UPLOAD_PATH")
	}
	if os.Getenv("AGENT_TOKEN") != "" {
		AgentToken = os.Getenv("AGENT_TOKEN")
	}
	if os.Getenv("DUSHENGCDN_AGENT_LEGACY_GLOBAL_TOKEN_ENABLED") != "" {
		AgentLegacyGlobalTokenEnabled = readBoolEnv("DUSHENGCDN_AGENT_LEGACY_GLOBAL_TOKEN_ENABLED")
	}
	if os.Getenv("REDIS_REQUIRED") != "" {
		RedisRequired = readBoolEnv("REDIS_REQUIRED")
	}
	if os.Getenv("DUSHENGCDN_LICENSE_REQUIRED") != "" {
		CommercialLicenseRequired = readBoolEnv("DUSHENGCDN_LICENSE_REQUIRED")
	}
	if os.Getenv("DUSHENGCDN_LICENSE_PUBLIC_KEYS") != "" {
		CommercialLicensePublicKeys = os.Getenv("DUSHENGCDN_LICENSE_PUBLIC_KEYS")
	}
	if os.Getenv("DUSHENGCDN_LICENSE_ALLOW_UNSIGNED") != "" {
		CommercialLicenseAllowUnsigned = readBoolEnv("DUSHENGCDN_LICENSE_ALLOW_UNSIGNED")
	}
	if os.Getenv("DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY") != "" {
		CommercialLicenseIssuerPrivateKey = os.Getenv("DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY")
	}
	if os.Getenv("DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY_FILE") != "" {
		CommercialLicenseIssuerPrivateKeyFile = os.Getenv("DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY_FILE")
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_LICENSE_ACTIVATION_URL")) != "" {
		CommercialLicenseActivationURL = strings.TrimSpace(os.Getenv("DUSHENGCDN_LICENSE_ACTIVATION_URL"))
	}
	if os.Getenv("DUSHENGCDN_LICENSE_ONLINE_ACTIVATION_REQUIRED") != "" {
		CommercialLicenseOnlineActivationRequired = readBoolEnv("DUSHENGCDN_LICENSE_ONLINE_ACTIVATION_REQUIRED")
	}
	if os.Getenv("DUSHENGCDN_LICENSE_ACTIVATION_SERVER_ENABLED") != "" {
		CommercialLicenseActivationServerEnabled = readBoolEnv("DUSHENGCDN_LICENSE_ACTIVATION_SERVER_ENABLED")
	}
	if value := readPositiveIntEnv("DUSHENGCDN_LICENSE_LEASE_DURATION_HOURS"); value > 0 {
		CommercialLicenseLeaseDuration = time.Duration(value) * time.Hour
	}
	if value := readPositiveIntEnv("DUSHENGCDN_LICENSE_LEASE_RENEW_BEFORE_HOURS"); value > 0 {
		CommercialLicenseLeaseRenewBefore = time.Duration(value) * time.Hour
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_BUILD_WATERMARK")) != "" {
		CommercialBuildWatermark = strings.TrimSpace(os.Getenv("DUSHENGCDN_BUILD_WATERMARK"))
	}
	enforceCommercialBuildMode()
	if os.Getenv("DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED") != "" {
		ServerAutoUpgradeEnabled = readBoolEnv("DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED")
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_SERVER_UPDATE_REPO")) != "" {
		ServerUpdateRepo = strings.TrimSpace(os.Getenv("DUSHENGCDN_SERVER_UPDATE_REPO"))
	}
	if os.Getenv("DUSHENGCDN_GITHUB_RELEASE_TOKEN") != "" {
		GitHubReleaseToken = os.Getenv("DUSHENGCDN_GITHUB_RELEASE_TOKEN")
	} else if os.Getenv("GITHUB_TOKEN") != "" {
		GitHubReleaseToken = os.Getenv("GITHUB_TOKEN")
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_MIRROR_PATH")) != "" {
		DNSSourceDatabaseMirrorPath = strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_MIRROR_PATH"))
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_COUNTRY_URL")) != "" {
		DNSSourceDatabaseCountryURL = strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_COUNTRY_URL"))
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_ASN_URL")) != "" {
		DNSSourceDatabaseASNURL = strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_ASN_URL"))
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_OPERATOR_CIDR_BASE_URL")) != "" {
		DNSSourceDatabaseOperatorCIDRBaseURL = strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_OPERATOR_CIDR_BASE_URL"))
	}
	if strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_OPERATOR_CIDR_FILES")) != "" {
		DNSSourceDatabaseOperatorCIDRFiles = strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_SOURCE_DATABASE_OPERATOR_CIDR_FILES"))
	}
	SetLogLevel(os.Getenv("LOG_LEVEL"))
	if *LogDir != "" {
		var err error
		*LogDir, err = filepath.Abs(*LogDir)
		if err != nil {
			slog.Error("resolve log directory failed", "error", err)
			os.Exit(1)
		}
		if _, err := os.Stat(*LogDir); os.IsNotExist(err) {
			err = os.Mkdir(*LogDir, 0750)
			if err != nil {
				slog.Error("create log directory failed", "error", err)
				os.Exit(1)
			}
		}
	}
	if _, err := os.Stat(UploadPath); os.IsNotExist(err) {
		_ = os.Mkdir(UploadPath, 0750)
	}
}

func readPositiveIntEnv(key string) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		slog.Warn("ignore invalid positive integer environment value", "key", key, "value", raw)
		return 0
	}
	return value
}

func readPositiveInt64Env(key string) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		slog.Warn("ignore invalid positive integer environment value", "key", key, "value", raw)
		return 0
	}
	return value
}

func readBoolEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func enforceCommercialBuildMode() {
	if strings.EqualFold(strings.TrimSpace(CommercialBuildMode), "required-online") {
		CommercialLicenseRequired = true
		CommercialLicenseOnlineActivationRequired = true
		CommercialLicenseAllowUnsigned = false
	}
}
