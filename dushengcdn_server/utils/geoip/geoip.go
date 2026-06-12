package geoip

import (
	"dushengcdn/common"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"

	ristretto "github.com/dgraph-io/ristretto/v2"
)

var CurrentProvider GeoIPService
var geoCache *providerCache
var providerMutex sync.RWMutex
var providerFactory = newProvider

const (
	ProviderDisabled = "disabled"
	ProviderMaxMind  = "mmdb"
	ProviderIPAPI    = "ip-api"
	ProviderGeoJS    = "geojs"
	ProviderIPInfo   = "ipinfo"

	providerCacheNearMax = 4096
)

type GeoInfo struct {
	ISOCode   string
	Name      string
	Operator  string
	Latitude  *float64
	Longitude *float64
}

func init() {
	CurrentProvider = &EmptyProvider{}
	geoCache = newProviderCache(48 * time.Hour)
}

// GeoIPService 接口定义了获取地理位置信息的核心方法。
type GeoIPService interface {
	Name() string
	GetGeoInfo(ip net.IP) (*GeoInfo, error)
	UpdateDatabase() error
	Close() error
}

type cachedGeoInfo struct {
	info      *GeoInfo
	expiresAt time.Time
}

type providerCache struct {
	items     *ristretto.Cache[string, cachedGeoInfo]
	duration  time.Duration
	nearMu    sync.RWMutex
	nearItems map[string]cachedGeoInfo
}

func newProviderCache(duration time.Duration) *providerCache {
	items, err := ristretto.NewCache(&ristretto.Config[string, cachedGeoInfo]{
		NumCounters: 1e5,
		MaxCost:     2e4,
		BufferItems: 64,
	})
	if err != nil {
		slog.Warn("GeoIP cache disabled", "error", err)
		return &providerCache{
			duration: duration,
		}
	}
	return &providerCache{
		items:     items,
		duration:  duration,
		nearItems: make(map[string]cachedGeoInfo),
	}
}

func (c *providerCache) Get(key string) (*GeoInfo, bool) {
	if c == nil || c.items == nil {
		return nil, false
	}
	if entry, ok := c.getNear(key); ok {
		return entry.info, true
	}
	entry, ok := c.items.Get(key)
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.items.Del(key)
		c.deleteNear(key)
		return nil, false
	}
	c.setNear(key, entry)
	return entry.info, true
}

func (c *providerCache) Set(key string, info *GeoInfo) {
	if c == nil || c.items == nil {
		return
	}
	entry := cachedGeoInfo{
		info:      info,
		expiresAt: time.Now().Add(c.duration),
	}
	c.setNear(key, entry)
	c.items.Set(key, entry, 1)
}

func (c *providerCache) Flush() {
	if c == nil || c.items == nil {
		return
	}
	c.nearMu.Lock()
	c.nearItems = make(map[string]cachedGeoInfo)
	c.nearMu.Unlock()
	c.items.Clear()
}

func (c *providerCache) getNear(key string) (cachedGeoInfo, bool) {
	c.nearMu.RLock()
	entry, ok := c.nearItems[key]
	c.nearMu.RUnlock()
	if !ok {
		return cachedGeoInfo{}, false
	}
	if time.Now().After(entry.expiresAt) {
		c.deleteNear(key)
		return cachedGeoInfo{}, false
	}
	return entry, true
}

func (c *providerCache) setNear(key string, entry cachedGeoInfo) {
	c.nearMu.Lock()
	if c.nearItems == nil {
		c.nearItems = make(map[string]cachedGeoInfo)
	}
	if _, exists := c.nearItems[key]; !exists && len(c.nearItems) >= providerCacheNearMax {
		for oldKey := range c.nearItems {
			delete(c.nearItems, oldKey)
			break
		}
	}
	c.nearItems[key] = entry
	c.nearMu.Unlock()
}

func (c *providerCache) deleteNear(key string) {
	c.nearMu.Lock()
	delete(c.nearItems, key)
	c.nearMu.Unlock()
}

func GetRegionUnicodeEmoji(isoCode string) string {
	if len(isoCode) != 2 {
		return ""
	}
	isoCode = strings.ToUpper(isoCode)

	if !unicode.IsLetter(rune(isoCode[0])) || !unicode.IsLetter(rune(isoCode[1])) {
		return ""
	}

	rune1 := rune(0x1F1E6 + (rune(isoCode[0]) - 'A'))
	rune2 := rune(0x1F1E6 + (rune(isoCode[1]) - 'A'))
	return string(rune1) + string(rune2)
}

func InitGeoIP() {
	providerName := normalizeProvider(common.GeoIPProvider)
	nextProvider, err := providerFactory(providerName)
	if err != nil {
		slog.Error("initialize GeoIP provider failed", "provider", providerName, "error", err)
		nextProvider = &EmptyProvider{}
	}
	setProvider(nextProvider)
	if providerName == ProviderDisabled {
		slog.Info("GeoIP provider disabled")
		return
	}
	slog.Info("GeoIP provider configured", "provider", CurrentProvider.Name())
}

func GetGeoInfo(ip net.IP) (*GeoInfo, error) {
	if ip == nil {
		return nil, fmt.Errorf("IP address cannot be nil")
	}
	provider := getProvider()
	cacheKey := provider.Name() + ":" + ip.String()

	if cachedInfo, found := geoCache.Get(cacheKey); found {
		return cachedInfo, nil
	}

	info, err := provider.GetGeoInfo(ip)
	if err == nil && info != nil {
		geoCache.Set(cacheKey, info)
	}
	return info, err
}

func LookupGeoInfoWithProvider(providerName string, ip net.IP) (*GeoInfo, error) {
	if ip == nil {
		return nil, fmt.Errorf("IP address cannot be nil")
	}

	provider, err := providerFactory(normalizeProvider(providerName))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := provider.Close(); closeErr != nil {
			slog.Warn("close temporary GeoIP provider failed", "provider", provider.Name(), "error", closeErr)
		}
	}()

	return provider.GetGeoInfo(ip)
}

func UpdateDatabase() error {
	err := getProvider().UpdateDatabase()
	if err == nil {
		geoCache.Flush()
		slog.Info("GeoIP cache cleared due to database update.")
	}
	return err
}

func IsValidProvider(provider string) bool {
	switch normalizeProvider(provider) {
	case ProviderDisabled, ProviderMaxMind, ProviderIPAPI, ProviderGeoJS, ProviderIPInfo:
		return true
	default:
		return false
	}
}

func normalizeProvider(provider string) string {
	normalized := strings.TrimSpace(strings.ToLower(provider))
	if normalized == "" {
		return ProviderDisabled
	}
	return normalized
}

func newProvider(provider string) (GeoIPService, error) {
	switch provider {
	case ProviderDisabled:
		return &EmptyProvider{}, nil
	case ProviderMaxMind:
		return NewMaxMindGeoIPService()
	case ProviderIPAPI:
		return NewIPAPIService()
	case ProviderGeoJS:
		return NewGeoJSService()
	case ProviderIPInfo:
		return NewIPInfoService()
	default:
		return nil, fmt.Errorf("unsupported GeoIP provider %q", provider)
	}
}

func setProvider(provider GeoIPService) {
	providerMutex.Lock()
	previous := CurrentProvider
	CurrentProvider = provider
	providerMutex.Unlock()
	geoCache.Flush()
	if previous != nil && previous != provider {
		if err := previous.Close(); err != nil {
			slog.Warn("close previous GeoIP provider failed", "error", err)
		}
	}
}

func getProvider() GeoIPService {
	providerMutex.RLock()
	defer providerMutex.RUnlock()
	if CurrentProvider == nil {
		return &EmptyProvider{}
	}
	return CurrentProvider
}

func float64Pointer(value float64) *float64 {
	return &value
}

func ProviderFactoryForTest() func(string) (GeoIPService, error) {
	return providerFactory
}

func SetProviderFactoryForTest(factory func(string) (GeoIPService, error)) {
	if factory == nil {
		providerFactory = newProvider
		return
	}
	providerFactory = factory
}
