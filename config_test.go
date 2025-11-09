package mocrelay

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Test loading config without YAML file
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify default values
	if config.ServerAddr != "0.0.0.0:8234" {
		t.Errorf("Expected ServerAddr to be '0.0.0.0:8234', got '%s'", config.ServerAddr)
	}
	if config.ServerSendTimeout != 10*time.Second {
		t.Errorf("Expected ServerSendTimeout to be 10s, got %v", config.ServerSendTimeout)
	}
	if config.CacheSize != 100 {
		t.Errorf("Expected CacheSize to be 100, got %d", config.CacheSize)
	}
	if config.SQLiteEnabled != true {
		t.Errorf("Expected SQLiteEnabled to be true, got %v", config.SQLiteEnabled)
	}
	if config.NIP11.Name != "mocrelay" {
		t.Errorf("Expected NIP11.Name to be 'mocrelay', got '%s'", config.NIP11.Name)
	}
}

func TestLoadConfigWithEnvOverride(t *testing.T) {
	// Set environment variable
	os.Setenv("MOCRELAY_SERVER_ADDR", "127.0.0.1:9999")
	os.Setenv("MOCRELAY_CACHE_SIZE", "200")
	os.Setenv("MOCRELAY_SQLITE_ENABLED", "false")
	defer func() {
		os.Unsetenv("MOCRELAY_SERVER_ADDR")
		os.Unsetenv("MOCRELAY_CACHE_SIZE")
		os.Unsetenv("MOCRELAY_SQLITE_ENABLED")
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify environment variable overrides
	if config.ServerAddr != "127.0.0.1:9999" {
		t.Errorf("Expected ServerAddr to be '127.0.0.1:9999', got '%s'", config.ServerAddr)
	}
	if config.CacheSize != 200 {
		t.Errorf("Expected CacheSize to be 200, got %d", config.CacheSize)
	}
	if config.SQLiteEnabled != false {
		t.Errorf("Expected SQLiteEnabled to be false, got %v", config.SQLiteEnabled)
	}
}

func TestLoadNIP11FromYAML(t *testing.T) {
	// Test loading from test YAML file
	nip11, err := LoadNIP11FromYAML("nip11.example.yaml")
	if err != nil {
		t.Fatalf("LoadNIP11FromYAML failed: %v", err)
	}

	// Verify values from YAML
	if nip11.Name != "mocrelay" {
		t.Errorf("Expected Name to be 'mocrelay', got '%s'", nip11.Name)
	}
	if nip11.Description != "moctane's nostr relay" {
		t.Errorf("Expected Description to be 'moctane's nostr relay', got '%s'", nip11.Description)
	}
	if nip11.Contact != "admin@example.com" {
		t.Errorf("Expected Contact to be 'admin@example.com', got '%s'", nip11.Contact)
	}
	if len(nip11.SupportedNIPs) != 4 {
		t.Errorf("Expected 4 supported NIPs, got %d", len(nip11.SupportedNIPs))
	}

	// Verify limitation values
	if nip11.Limitation == nil {
		t.Fatal("Expected Limitation to be non-nil")
	}
	if nip11.Limitation.MaxMessageLength != 100000 {
		t.Errorf("Expected MaxMessageLength to be 100000, got %d", nip11.Limitation.MaxMessageLength)
	}
	if nip11.Limitation.MaxSubscriptions != 10 {
		t.Errorf("Expected MaxSubscriptions to be 10, got %d", nip11.Limitation.MaxSubscriptions)
	}
}

func TestLoadNIP11FromYAMLNotFound(t *testing.T) {
	// Test loading from non-existent file
	_, err := LoadNIP11FromYAML("nonexistent.yaml")
	if err == nil {
		t.Error("Expected error when loading non-existent file, got nil")
	}
}

func TestLimitationEnvOverride(t *testing.T) {
	// Set NIP-11 file and limitation overrides
	os.Setenv("MOCRELAY_NIP11_FILE", "nip11.test.yaml")
	os.Setenv("MOCRELAY_LIMITATION_MAX_MESSAGE_LENGTH", "200000")
	os.Setenv("MOCRELAY_LIMITATION_MAX_SUBSCRIPTIONS", "20")
	defer func() {
		os.Unsetenv("MOCRELAY_NIP11_FILE")
		os.Unsetenv("MOCRELAY_LIMITATION_MAX_MESSAGE_LENGTH")
		os.Unsetenv("MOCRELAY_LIMITATION_MAX_SUBSCRIPTIONS")
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify environment variable overrides limitation values from YAML
	if config.NIP11.Limitation.MaxMessageLength != 200000 {
		t.Errorf("Expected MaxMessageLength to be 200000, got %d", config.NIP11.Limitation.MaxMessageLength)
	}
	if config.NIP11.Limitation.MaxSubscriptions != 20 {
		t.Errorf("Expected MaxSubscriptions to be 20, got %d", config.NIP11.Limitation.MaxSubscriptions)
	}
}
