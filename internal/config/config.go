// Package config loads and validates O configuration.
//
// Resolution order:
//  1. path in the O_CONFIG environment variable
//  2. config/o.config.json found by walking up from the working directory
//  3. .o/config.json found by walking up from the working directory (merged on top)
//  4. built-in defaults
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Domains struct {
	Website    string `json:"website"`
	API        string `json:"api"`
	WebsiteURL string `json:"websiteUrl"`
	APIURL     string `json:"apiUrl"`
}

type Paths struct {
	SessionsDir string `json:"sessionsDir"`
	MemoryFile  string `json:"memoryFile"`
}

type ProviderConfig struct {
	BaseURL    string `json:"baseUrl"`
	Model      string `json:"model"`
	APIKeyEnv  string `json:"apiKeyEnv"`
	APIKey     string `json:"apiKey,omitempty"`
}

type AgentConfig struct {
	Description string         `json:"description,omitempty"`
	Model       string         `json:"model,omitempty"`
	Mode        string         `json:"mode,omitempty"`
	MaxTurns    int            `json:"maxTurns,omitempty"`
	Tools       map[string]any `json:"tools,omitempty"`
}

type ContextConfig struct {
	MaxTokens         int     `json:"maxTokens"`
	CompactThreshold  float64 `json:"compactThreshold"`
	MaxToolOutputChars int    `json:"maxToolOutputChars"`
}

type Config struct {
	Name             string                    `json:"name"`
	Company          string                    `json:"company"`
	Version          string                    `json:"version"`
	Domains          Domains                   `json:"domains"`
	Paths            Paths                     `json:"paths"`
	DefaultProvider  string                    `json:"defaultProvider"`
	Providers        map[string]ProviderConfig `json:"providers"`
	Agents           map[string]AgentConfig    `json:"agents"`
	Permissions      PermissionsConfig         `json:"permissions"`
	Context          ContextConfig             `json:"context"`
	MCPServers       map[string]any            `json:"mcpServers"`
	Hooks            map[string][]string       `json:"hooks"`
	Planner          string                    `json:"planner"`
	TUI              TUIConfig                 `json:"tui"`
}

// TUIConfig holds the terminal UI preferences that can be set in any O
// config file (shipped, user, project or $O_CONFIG).
type TUIConfig struct {
	// Theme is "dark" or "light".
	Theme string `json:"theme"`
	// RefreshHz is the UI refresh rate: 60, 120, 144 or 440.
	RefreshHz int `json:"refreshHz"`
}

type PermissionsConfig struct {
	Mode  string         `json:"mode"`
	Rules map[string]any `json:"rules"`
}

func Default() Config {
	return Config{
		Name:    "O",
		Company: "Adaop",
		Version: "0.0.0",
		Domains: Domains{
			Website: "adaop.sbs", API: "api.adaop.sbs",
			WebsiteURL: "https://adaop.sbs", APIURL: "https://api.adaop.sbs",
		},
		Paths:           Paths{SessionsDir: "~/.o/sessions", MemoryFile: "O.md"},
		DefaultProvider: "adaop",
		Providers: map[string]ProviderConfig{
			"adaop": {BaseURL: "https://api.adaop.sbs/v1", Model: "o-1", APIKeyEnv: "ADAOP_API_KEY"},
		},
		Agents: map[string]AgentConfig{
			"primary": {Model: "o-1", Mode: "default", MaxTurns: 50},
		},
		Permissions: PermissionsConfig{Mode: "default", Rules: map[string]any{}},
		Context:     ContextConfig{MaxTokens: 180000, CompactThreshold: 0.8, MaxToolOutputChars: 20000},
		MCPServers:  map[string]any{},
		Hooks:       map[string][]string{},
		Planner:     "mcts",
	}
}

func ValidMode(mode string) bool {
	switch mode {
	case "default", "auto-edit", "plan", "full-auto":
		return true
	}
	return false
}

// UserConfigPath is the user-level config file: $O_USER_CONFIG or ~/.o/config.json.
func UserConfigPath() string {
	if p := os.Getenv("O_USER_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".o", "config.json")
	}
	return filepath.Join(home, ".o", "config.json")
}

// Load resolves the effective config for the given working directory.
// Precedence (lowest to highest): built-in defaults < shipped config/o.config.json
// < user ~/.o/config.json < project .o/config.json < $O_CONFIG (replaces all).
func Load(workDir string) (Config, error) {
	cfg := Default()
	if p := os.Getenv("O_CONFIG"); p != "" {
		if err := mergeFile(&cfg, p); err != nil {
			return cfg, err
		}
		return validate(cfg)
	}
	if p, ok := findUp(workDir, filepath.Join("config", "o.config.json")); ok {
		if err := mergeFile(&cfg, p); err != nil {
			return cfg, err
		}
	}
	if p := UserConfigPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			if err := mergeFile(&cfg, p); err != nil {
				return cfg, err
			}
		}
	}
	if p, ok := findUp(workDir, filepath.Join(".o", "config.json")); ok {
		if err := mergeFile(&cfg, p); err != nil {
			return cfg, err
		}
	}
	return validate(cfg)
}

func loadRaw(path string) (map[string]any, error) {
	raw := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return raw, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return raw, nil
}

// saveRaw writes config JSON with owner-only permissions (it may hold API keys).
func saveRaw(path string, raw map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

var providerIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// NormalizeBaseURL trims a provider base URL and strips a trailing
// "/chat/completions" so the client can safely append it (a doubled suffix
// causes HTTP 404).
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "/")
	raw = strings.TrimSuffix(raw, "/chat/completions")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("base URL must be a valid http(s) URL like https://api.openai.com/v1 (got %q)", raw)
	}
	return raw, nil
}

// AddProvider writes a provider into the user-level config file (creating it),
// merging with whatever is already there. Returns the file path written.
func AddProvider(id string, pc ProviderConfig, setDefault bool) (string, error) {
	if !providerIDRe.MatchString(id) {
		return "", fmt.Errorf("provider id must be letters/digits/._- only (got %q)", id)
	}
	base, err := NormalizeBaseURL(pc.BaseURL)
	if err != nil {
		return "", err
	}
	pc.BaseURL = base
	if pc.Model == "" {
		return "", errors.New("model is required")
	}
	path := UserConfigPath()
	raw, err := loadRaw(path)
	if err != nil {
		return "", err
	}
	providers, _ := raw["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	entry := map[string]any{"baseUrl": pc.BaseURL, "model": pc.Model}
	if pc.APIKey != "" {
		entry["apiKey"] = pc.APIKey
	}
	if pc.APIKeyEnv != "" {
		entry["apiKeyEnv"] = pc.APIKeyEnv
	}
	providers[id] = entry
	raw["providers"] = providers
	if setDefault {
		raw["defaultProvider"] = id
	}
	if err := saveRaw(path, raw); err != nil {
		return "", err
	}
	return path, nil
}

// RemoveProvider deletes a provider from the user-level config file.
func RemoveProvider(id string) (bool, error) {
	path := UserConfigPath()
	raw, err := loadRaw(path)
	if err != nil {
		return false, err
	}
	providers, _ := raw["providers"].(map[string]any)
	if providers == nil {
		return false, nil
	}
	if _, ok := providers[id]; !ok {
		return false, nil
	}
	delete(providers, id)
	raw["providers"] = providers
	if raw["defaultProvider"] == id {
		delete(raw, "defaultProvider")
	}
	return true, saveRaw(path, raw)
}

// SetDefaultProvider sets defaultProvider in the user-level config file.
func SetDefaultProvider(id string) (string, error) {
	path := UserConfigPath()
	raw, err := loadRaw(path)
	if err != nil {
		return "", err
	}
	raw["defaultProvider"] = id
	if err := saveRaw(path, raw); err != nil {
		return "", err
	}
	return path, nil
}

// ListProviders returns the ids of all configured providers (merged view).
func ListProviders(cfg Config) []string {
	ids := make([]string, 0, len(cfg.Providers))
	for id := range cfg.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func findUp(start, rel string) (string, bool) {
	dir := start
	for {
		p := filepath.Join(dir, rel)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func mergeFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config %s: %w", path, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	merged, err := deepMerge(toAny(cfg), raw)
	if err != nil {
		return err
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, cfg); err != nil {
		return fmt.Errorf("apply config %s: %w", path, err)
	}
	return nil
}

func toAny(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// deepMerge merges override into base; maps merge recursively, everything else replaces.
func deepMerge(base, override any) (any, error) {
	bm, bok := base.(map[string]any)
	om, ook := override.(map[string]any)
	if bok && ook {
		for k, v := range om {
			if bv, exists := bm[k]; exists {
				m, err := deepMerge(bv, v)
				if err != nil {
					return nil, err
				}
				bm[k] = m
			} else {
				bm[k] = v
			}
		}
		return bm, nil
	}
	return override, nil
}

func validate(cfg Config) (Config, error) {
	if cfg.Domains.Website == "" || cfg.Domains.API == "" {
		return cfg, errors.New("config.domains must define website and api")
	}
	if !ValidMode(cfg.Permissions.Mode) {
		return cfg, fmt.Errorf("config.permissions.mode must be default|auto-edit|plan|full-auto, got %q", cfg.Permissions.Mode)
	}
	if cfg.Context.MaxTokens <= 0 {
		cfg.Context.MaxTokens = 180000
	}
	if cfg.Context.CompactThreshold <= 0 || cfg.Context.CompactThreshold > 1 {
		cfg.Context.CompactThreshold = 0.8
	}
	if cfg.Context.MaxToolOutputChars <= 0 {
		cfg.Context.MaxToolOutputChars = 20000
	}
	return cfg, nil
}
