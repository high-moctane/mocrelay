package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

var DefaultConfig = &Config{
	Addr:             "127.0.0.1:8234",
	MonitorAddr:      "127.0.0.1:8396",
	CacheSize:        10000,
	MaxConnections:   1000000,
	MaxMessageLength: 1048576,
	MaxSubscriptions: 20,
	MaxFilters:       20,
	MaxSubIDLength:   64,
	TopPageMessage:   "Welcome to Mocrelay (｀･ω･´)",
}

func NewConfig(b []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cfg: %w", err)
	}

	if c.Addr == "" {
		c.Addr = DefaultConfig.Addr
	}

	if c.MonitorAddr == "" {
		c.MonitorAddr = DefaultConfig.MonitorAddr
	}

	if c.CacheSize == 0 {
		c.CacheSize = DefaultConfig.CacheSize
	}

	if c.MaxConnections == 0 {
		c.MaxConnections = DefaultConfig.MaxConnections
	}

	if c.MaxMessageLength == 0 {
		c.MaxMessageLength = DefaultConfig.MaxMessageLength
	}

	if c.MaxSubscriptions == 0 {
		c.MaxSubscriptions = DefaultConfig.MaxSubscriptions
	}

	if c.MaxFilters == 0 {
		c.MaxFilters = DefaultConfig.MaxFilters
	}

	if c.MaxSubIDLength > 64 {
		return nil, fmt.Errorf("too big max_subid_length: %v", c.MaxSubIDLength)
	}
	if c.MaxSubIDLength == 0 {
		c.MaxSubIDLength = DefaultConfig.MaxSubIDLength
	}

	if c.MinPrefix > 32 {
		return nil, fmt.Errorf("too big min_prefix: %v", c.MinPrefix)
	}

	if c.TopPageMessage == "" {
		c.TopPageMessage = DefaultConfig.TopPageMessage
	}

	return &c, nil
}

func NewConfigFromFilePath(fpath string) (*Config, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s", fpath)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("fail to read %s: %w", fpath, err)
	}

	return NewConfig(b)
}

type Config struct {
	Addr           string `json:"addr,omitempty"`
	MonitorAddr    string `json:"monitor_addr,omitempty"`
	CacheSize      int    `json:"cache_size,omitempty"`
	MaxConnections int    `json:"max_connections,omitempty"`

	// NIP11
	NIP11Name        *string `json:"nip11_name,omitempty"`
	NIP11Description *string `json:"nip11_description,omitempty"`
	NIP11Pubkey      *string `json:"nip11_pubkey,omitempty"`
	NIP11Contact     *string `json:"nip11_contact,omitempty"`

	MaxMessageLength int `json:"max_message_length,omitempty"`
	MaxSubscriptions int `json:"max_subscriptions,omitempty"`
	MaxFilters       int `json:"max_filters,omitempty"`
	MaxLimit         int `json:"max_limit,omitempty"`
	MaxSubIDLength   int `json:"max_subid_length,omitempty"`
	MinPrefix        int `json:"min_prefix,omitempty"`
	MaxEventTags     int `json:"max_event_tags,omitempty"`
	MaxContentLength int `json:"max_content_length,omitempty"`

	TopPageMessage string `json:"top_page_message,omitempty"`

	Icon string `json:"icon,omitempty"`
}
