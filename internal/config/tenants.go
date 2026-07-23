package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type TenantConfig struct {
	Domains []string `json:"domains"`
}

func LoadTenantConfig(path string) (*TenantConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tenant config %q: %w", path, err)
	}

	var cfg TenantConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse tenant config %q: %w", path, err)
	}

	cfg.normalize()
	if len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("tenant config %q contains no domains", path)
	}

	return &cfg, nil
}

func DefaultTenantConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("SPREZZ_TENANTS_CONFIG")); path != "" {
		return path
	}
	return "tenants.json"
}

func (c *TenantConfig) Contains(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return false
	}
	for _, item := range c.Domains {
		if strings.EqualFold(strings.TrimSpace(item), domain) {
			return true
		}
	}
	return false
}

func (c *TenantConfig) normalize() {
	seen := make(map[string]struct{}, len(c.Domains))
	cleaned := make([]string, 0, len(c.Domains))
	for _, domain := range c.Domains {
		domain = strings.TrimSpace(strings.ToLower(domain))
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		cleaned = append(cleaned, domain)
	}
	c.Domains = cleaned
}
