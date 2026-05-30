package geoip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdaterDownloadsCountryDatabase(t *testing.T) {
	payload := make([]byte, minDatabaseSize+16)
	for index := range payload {
		payload[index] = byte(index % 251)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	updater := &Updater{
		URL:      server.URL,
		Path:     target,
		Interval: time.Hour,
	}
	if err := updater.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("expected database file to exist: %v", err)
	}
	if info.Size() != int64(len(payload)) {
		t.Fatalf("unexpected database size: got %d want %d", info.Size(), len(payload))
	}
}

func TestUpdaterRejectsTinyDatabase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tiny"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	updater := &Updater{URL: server.URL, Path: target}
	if err := updater.Ensure(context.Background()); err == nil {
		t.Fatal("expected tiny database download to fail")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected tiny database not to be activated, stat err = %v", err)
	}
}
