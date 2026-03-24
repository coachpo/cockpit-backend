package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

func TestMainExitsNonZeroWhenBootstrapFails(t *testing.T) {
	if os.Getenv("COCKPIT_MAIN_FAIL_HELPER") == "1" {
		tempDir := t.TempDir()
		if err := os.Chdir(tempDir); err != nil {
			t.Fatalf("chdir temp dir: %v", err)
		}
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
		os.Args = []string{os.Args[0]}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainExitsNonZeroWhenBootstrapFails$")
	cmd.Env = append(os.Environ(),
		"COCKPIT_MAIN_FAIL_HELPER=1",
		"NACOS_ADDR=",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected child process to exit non-zero, output=%q", string(output))
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T (%v)", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, output=%q", string(output))
	}
	if !strings.Contains(string(output), "failed to bootstrap configuration") {
		t.Fatalf("expected bootstrap failure output, got %q", string(output))
	}
	if strings.Contains(string(output), "Cockpit Version:") {
		t.Fatalf("expected bootstrap failure output to omit build metadata, got %q", string(output))
	}
}

func TestNewCommandFlagSet_ExposesRuntimeOverrideFlags(t *testing.T) {
	var options commandOptions
	fs := newCommandFlagSet("cockpit", &options)
	for _, flagName := range []string{"host", "port"} {
		if fs.Lookup(flagName) == nil {
			t.Fatalf("expected -%s flag to exist", flagName)
		}
	}
}

func TestParseCommandArgs_RejectsPositionalArgs(t *testing.T) {
	_, err := parseCommandArgs("cockpit", []string{"/tmp/legacy-config.yaml"})
	if err == nil {
		t.Fatal("expected positional config path to be rejected")
	}
	if !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("expected positional-arg rejection, got %q", err)
	}
}

func TestParseCommandArgs_ParsesRuntimeOverrides(t *testing.T) {
	options, err := parseCommandArgs("cockpit", []string{"--host", "0.0.0.0", "--port", "8080"})
	if err != nil {
		t.Fatalf("parseCommandArgs() error = %v", err)
	}
	if !options.hostSet || options.host != "0.0.0.0" {
		t.Fatalf("expected host override to be set, got host=%q hostSet=%v", options.host, options.hostSet)
	}
	if !options.portSet || options.port != 8080 {
		t.Fatalf("expected port override to be set, got port=%d portSet=%v", options.port, options.portSet)
	}
}

func TestParseCommandArgs_RejectsBlankHostOverride(t *testing.T) {
	_, err := parseCommandArgs("cockpit", []string{"--host", "   "})
	if err == nil {
		t.Fatal("expected error for blank host override")
	}
	if !strings.Contains(err.Error(), "host flag") {
		t.Fatalf("expected host validation error, got %q", err)
	}
}

func TestParseCommandArgs_RejectsOutOfRangePortOverride(t *testing.T) {
	_, err := parseCommandArgs("cockpit", []string{"--port", "70000"})
	if err == nil {
		t.Fatal("expected error for out-of-range port override")
	}
	if !strings.Contains(err.Error(), "port flag") {
		t.Fatalf("expected port validation error, got %q", err)
	}
}

func TestApplyRuntimeOverrides_OverridesOnlyProvidedValues(t *testing.T) {
	cfg := &config.Config{Host: "127.0.0.1", Port: 8317}
	if err := applyRuntimeOverrides(cfg, commandOptions{host: "0.0.0.0", hostSet: true}); err != nil {
		t.Fatalf("applyRuntimeOverrides() host override error = %v", err)
	}
	if cfg.Host != "0.0.0.0" || cfg.Port != 8317 {
		t.Fatalf("expected only host override to apply, got host=%q port=%d", cfg.Host, cfg.Port)
	}
	if err := applyRuntimeOverrides(cfg, commandOptions{port: 8080, portSet: true}); err != nil {
		t.Fatalf("applyRuntimeOverrides() port override error = %v", err)
	}
	if cfg.Host != "0.0.0.0" || cfg.Port != 8080 {
		t.Fatalf("expected both overrides to be preserved, got host=%q port=%d", cfg.Host, cfg.Port)
	}
}

func TestResolveBootstrapConfig_UsesNacos(t *testing.T) {
	result, err := resolveBootstrapConfig(bootstrapLoaders{
		nacosAddr: "127.0.0.1:8848",
		loadNacos: func() (*bootstrapConfig, error) {
			return &bootstrapConfig{
				cfg:          &config.Config{Port: 8080},
				configSource: &nacos.NacosConfigStore{},
				authStore:    &nacos.NacosAuthStore{},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("resolveBootstrapConfig() error = %v", err)
	}
	if result == nil {
		t.Fatal("resolveBootstrapConfig() returned nil result")
	}
	if result.configSource.Mode() != "nacos" {
		t.Fatalf("expected nacos config source, got %q", result.configSource.Mode())
	}
	if result.authStore == nil {
		t.Fatal("expected nacos auth store to be preserved")
	}
}

func TestResolveBootstrapConfig_FailsFastWhenNacosFails(t *testing.T) {
	_, err := resolveBootstrapConfig(bootstrapLoaders{
		nacosAddr: "127.0.0.1:8848",
		loadNacos: func() (*bootstrapConfig, error) {
			return nil, errors.New("nacos unavailable")
		},
	})
	if err == nil {
		t.Fatal("expected error when nacos bootstrap fails")
	}
	if !strings.Contains(err.Error(), "failed to bootstrap from nacos") || !strings.Contains(err.Error(), "nacos unavailable") {
		t.Fatalf("expected nacos bootstrap error, got %q", err)
	}
}

func TestBootstrapFromNacosStores_LoadsConfigAndAuths(t *testing.T) {
	wantCfg := &config.Config{Port: 8080}

	result, err := bootstrapFromNacosStores(
		&nacos.NacosConfigStore{},
		&nacos.NacosAuthStore{},
		func() (*config.Config, error) { return wantCfg, nil },
		func(context.Context) ([]*coreauth.Auth, error) {
			return []*coreauth.Auth{{ID: "codex.json"}}, nil
		},
	)
	if err != nil {
		t.Fatalf("bootstrapFromNacosStores() error = %v", err)
	}
	if result == nil {
		t.Fatal("bootstrapFromNacosStores() returned nil result")
	}
	if result.cfg != wantCfg {
		t.Fatalf("expected bootstrap config pointer to be preserved, got %#v", result.cfg)
	}
	if result.configSource == nil || result.authStore == nil {
		t.Fatal("expected bootstrap result to preserve nacos stores")
	}
}

func TestBootstrapFromNacosStores_FailsWhenAuthLoadFails(t *testing.T) {
	_, err := bootstrapFromNacosStores(
		&nacos.NacosConfigStore{},
		&nacos.NacosAuthStore{},
		func() (*config.Config, error) { return &config.Config{}, nil },
		func(context.Context) ([]*coreauth.Auth, error) {
			return nil, errors.New("nacos auths unavailable")
		},
	)
	if err == nil {
		t.Fatal("expected error when auth bootstrap fails")
	}
	if !strings.Contains(err.Error(), "load auths from nacos") || !strings.Contains(err.Error(), "nacos auths unavailable") {
		t.Fatalf("expected auth bootstrap error, got %q", err)
	}
}

func TestResolveBootstrapConfig_FailsWhenNacosAddrIsMissing(t *testing.T) {
	nacosCalls := 0

	_, err := resolveBootstrapConfig(bootstrapLoaders{
		loadNacos: func() (*bootstrapConfig, error) {
			nacosCalls++
			return &bootstrapConfig{
				cfg:          &config.Config{},
				configSource: &nacos.NacosConfigStore{},
				authStore:    &nacos.NacosAuthStore{},
			}, nil
		},
	})
	if err == nil {
		t.Fatal("expected error when NACOS_ADDR is missing")
	}
	if !strings.Contains(err.Error(), "NACOS_ADDR") {
		t.Fatalf("expected missing NACOS_ADDR error, got %q", err)
	}
	if nacosCalls != 0 {
		t.Fatalf("expected nacos loader not to run without NACOS_ADDR, got %d call(s)", nacosCalls)
	}
}

func TestResolveBootstrapConfig_FailsWhenBootstrapAuthStoreIsMissing(t *testing.T) {
	_, err := resolveBootstrapConfig(bootstrapLoaders{
		nacosAddr: "127.0.0.1:8848",
		loadNacos: func() (*bootstrapConfig, error) {
			return &bootstrapConfig{
				cfg:          &config.Config{},
				configSource: &nacos.NacosConfigStore{},
			}, nil
		},
	})
	if err == nil {
		t.Fatal("expected error when bootstrap auth store is missing")
	}
	if !strings.Contains(err.Error(), "auth store") {
		t.Fatalf("expected missing auth store error, got %q", err)
	}
}

func TestResolveBootstrapConfig_FailsWhenBootstrapConfigSourceIsMissing(t *testing.T) {
	_, err := resolveBootstrapConfig(bootstrapLoaders{
		nacosAddr: "127.0.0.1:8848",
		loadNacos: func() (*bootstrapConfig, error) {
			return &bootstrapConfig{
				cfg:       &config.Config{},
				authStore: &nacos.NacosAuthStore{},
			}, nil
		},
	})
	if err == nil {
		t.Fatal("expected error when bootstrap config source is missing")
	}
	if !strings.Contains(err.Error(), "config source") {
		t.Fatalf("expected missing config source error, got %q", err)
	}
}
