package security

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type verificationValue struct {
	code string
	time time.Time
}

const (
	EmailVerificationPurpose = "v"
	PasswordResetPurpose     = "r"
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationMapMaxSize = 10000
var VerificationValidMinutes = 10

func GenerateVerificationCode(length int) string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	if length == 0 {
		return code
	}
	return code[:length]
}

func RegisterVerificationCodeWithKey(key string, code string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	now := time.Now()
	verificationKey := purpose + key
	removeExpiredPairsBefore(now)
	if _, exists := verificationMap[verificationKey]; !exists && verificationMapMaxSize > 0 && len(verificationMap) >= verificationMapMaxSize {
		removeOldestPairs(len(verificationMap) - verificationMapMaxSize + 1)
	}
	verificationMap[verificationKey] = verificationValue{
		code: code,
		time: now,
	}
}

func VerifyCodeWithKey(key string, code string, purpose string) bool {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	value, okay := verificationMap[purpose+key]
	now := time.Now()
	if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		if okay {
			delete(verificationMap, purpose+key)
		}
		return false
	}
	return code == value.code
}

func VerifyCodeWithKeyAndDelete(key string, code string, purpose string) bool {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationKey := purpose + key
	value, okay := verificationMap[verificationKey]
	now := time.Now()
	if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		if okay {
			delete(verificationMap, verificationKey)
		}
		return false
	}
	if code != value.code {
		return false
	}
	delete(verificationMap, verificationKey)
	return true
}

func DeleteKey(key string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	delete(verificationMap, purpose+key)
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeExpiredPairs() {
	removeExpiredPairsBefore(time.Now())
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeExpiredPairsBefore(now time.Time) {
	expiration := time.Duration(VerificationValidMinutes) * time.Minute
	if expiration <= 0 {
		verificationMap = make(map[string]verificationValue)
		return
	}
	for key, value := range verificationMap {
		if now.Sub(value.time) >= expiration {
			delete(verificationMap, key)
		}
	}
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeOldestPairs(count int) {
	for i := 0; i < count; i++ {
		oldestKey := ""
		var oldestTime time.Time
		for key, value := range verificationMap {
			if oldestKey == "" || value.time.Before(oldestTime) {
				oldestKey = key
				oldestTime = value.time
			}
		}
		if oldestKey == "" {
			return
		}
		delete(verificationMap, oldestKey)
	}
}

func init() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
