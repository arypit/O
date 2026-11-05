package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAddListRemoveProvider(t *testing.T) {
	tmp := t.TempDir()
	userCfg := filepath.Join(tmp, "config.json")
	t.Setenv("O_USER_CONFIG", userCfg)
	// O_CONFIG replaces the file search entirely, so the real ~/.o/config.json
	// up the temp-dir path can never leak into the test
	t.Setenv("O_CONFIG", userCfg)

	path, err := AddProvider("myllm", ProviderConfig{BaseURL: "https://llm.example.com/v1", Model: "my-model-7b", APIKey: "secret123"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if path != userCfg {
		t.Errorf("path = %s, want %s", path, userCfg)
	}
	// file must be owner-only (Unix permission bits; Windows only has the read-only bit)
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(userCfg)
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("perm = %o, want 600", fi.Mode().Perm())
		}
	}
	cfg, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	pc, ok := cfg.Providers["myllm"]
	if !ok || pc.BaseURL != "https://llm.example.com/v1" || pc.Model != "my-model-7b" || pc.APIKey != "secret123" {
		t.Errorf("provider = %+v", pc)
	}
	if cfg.DefaultProvider != "myllm" {
		t.Errorf("defaultProvider = %q", cfg.DefaultProvider)
	}
	if ids := ListProviders(cfg); len(ids) < 2 { // adaop default + myllm
		t.Errorf("providers = %v", ids)
	}
	ok, err = RemoveProvider("myllm")
	if err != nil || !ok {
		t.Fatalf("remove = %v, %v", ok, err)
	}
	cfg, _ = Load(tmp)
	if _, ok := cfg.Providers["myllm"]; ok {
		t.Error("provider should be gone after remove")
	}
	if cfg.DefaultProvider == "myllm" {
		t.Error("defaultProvider should be cleared")
	}
}

func TestAddProviderValidation(t *testing.T) {
	t.Setenv("O_USER_CONFIG", filepath.Join(t.TempDir(), "c.json"))
	t.Setenv("O_CONFIG", filepath.Join(t.TempDir(), "c2.json"))
	if _, err := AddProvider("", ProviderConfig{BaseURL: "x", Model: "m"}, false); err == nil {
		t.Error("empty id must fail")
	}
	if _, err := AddProvider("bad id/with slash", ProviderConfig{BaseURL: "https://x", Model: "m"}, false); err == nil {
		t.Error("id with spaces/slashes must fail")
	}
	if _, err := AddProvider("x", ProviderConfig{Model: "m"}, false); err == nil {
		t.Error("empty baseUrl must fail")
	}
	if _, err := AddProvider("x", ProviderConfig{BaseURL: "notaurl", Model: "m"}, false); err == nil {
		t.Error("non-URL baseUrl must fail")
	}
	if _, err := AddProvider("x", ProviderConfig{BaseURL: "https://x"}, false); err == nil {
		t.Error("empty model must fail")
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1":                          "https://api.openai.com/v1",
		"https://api.openai.com/v1/":                         "https://api.openai.com/v1",
		"https://integrate.api.nvidia.com/v1/chat/completions": "https://integrate.api.nvidia.com/v1",
		"http://127.0.0.1:11434/v1/":                         "http://127.0.0.1:11434/v1",
	}
	for in, want := range cases {
		got, err := NormalizeBaseURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "nvidia.com/v1", "ftp://x", "https://"} {
		if _, err := NormalizeBaseURL(bad); err == nil {
			t.Errorf("NormalizeBaseURL(%q) must fail", bad)
		}
	}
	// AddProvider stores the normalized URL
	tmp := t.TempDir()
	t.Setenv("O_USER_CONFIG", filepath.Join(tmp, "c.json"))
	t.Setenv("O_CONFIG", filepath.Join(tmp, "c.json"))
	if _, err := AddProvider("nvidia", ProviderConfig{BaseURL: "https://integrate.api.nvidia.com/v1/chat/completions", Model: "m"}, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers["nvidia"].BaseURL != "https://integrate.api.nvidia.com/v1" {
		t.Errorf("stored baseUrl = %q", cfg.Providers["nvidia"].BaseURL)
	}
}
