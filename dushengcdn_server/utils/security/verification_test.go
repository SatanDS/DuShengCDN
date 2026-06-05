package security

import (
	"testing"
	"time"
)

func TestVerificationCacheEvictsOldestWhenFull(t *testing.T) {
	resetVerificationMapForTest(t, 2, 10)

	RegisterVerificationCodeWithKey("old@example.com", "old", EmailVerificationPurpose)
	time.Sleep(time.Millisecond)
	RegisterVerificationCodeWithKey("new@example.com", "new", EmailVerificationPurpose)
	time.Sleep(time.Millisecond)
	RegisterVerificationCodeWithKey("latest@example.com", "latest", EmailVerificationPurpose)

	if VerifyCodeWithKey("old@example.com", "old", EmailVerificationPurpose) {
		t.Fatal("expected oldest verification code to be evicted")
	}
	if !VerifyCodeWithKey("new@example.com", "new", EmailVerificationPurpose) {
		t.Fatal("expected newer verification code to remain")
	}
	if !VerifyCodeWithKey("latest@example.com", "latest", EmailVerificationPurpose) {
		t.Fatal("expected latest verification code to remain")
	}
}

func TestVerificationCacheDeletesExpiredCodeOnVerify(t *testing.T) {
	resetVerificationMapForTest(t, 10, 0)

	RegisterVerificationCodeWithKey("expired@example.com", "expired", PasswordResetPurpose)
	if VerifyCodeWithKey("expired@example.com", "expired", PasswordResetPurpose) {
		t.Fatal("expected zero-minute verification code to be expired")
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	if _, ok := verificationMap[PasswordResetPurpose+"expired@example.com"]; ok {
		t.Fatal("expected expired verification code to be removed")
	}
}

func resetVerificationMapForTest(t *testing.T, maxSize int, validMinutes int) {
	t.Helper()
	verificationMutex.Lock()
	oldMap := verificationMap
	oldMaxSize := verificationMapMaxSize
	oldValidMinutes := VerificationValidMinutes
	verificationMap = make(map[string]verificationValue)
	verificationMapMaxSize = maxSize
	VerificationValidMinutes = validMinutes
	verificationMutex.Unlock()

	t.Cleanup(func() {
		verificationMutex.Lock()
		verificationMap = oldMap
		verificationMapMaxSize = oldMaxSize
		VerificationValidMinutes = oldValidMinutes
		verificationMutex.Unlock()
	})
}
