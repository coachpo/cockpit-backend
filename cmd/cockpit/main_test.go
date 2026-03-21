package main

import (
	"bytes"
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
	log "github.com/sirupsen/logrus"
)

type modeTestConfigSource struct {
	mode string
}

func (s modeTestConfigSource) LoadConfig() (*config.Config, error) { return &config.Config{}, nil }
func (s modeTestConfigSource) SaveConfig(*config.Config) error     { return nil }
func (s modeTestConfigSource) WatchConfig(func(*config.Config)) error {
	return nil
}
func (s modeTestConfigSource) StopWatch()   {}
func (s modeTestConfigSource) Mode() string { return s.mode }

type modeTestAuthStore struct{}

func (modeTestAuthStore) List(context.Context) ([]*coreauth.Auth, error) { return nil, nil }
func (modeTestAuthStore) Save(context.Context, *coreauth.Auth) (string, error) {
	return "", nil
}
func (modeTestAuthStore) Delete(context.Context, string) error { return nil }
func (modeTestAuthStore) ReadByName(context.Context, string) ([]byte, error) {
	return nil, nil
}
func (modeTestAuthStore) ListMetadata(context.Context) ([]nacos.AuthFileMetadata, error) {
	return nil, nil
}
func (modeTestAuthStore) Watch(context.Context, func([]*coreauth.Auth)) error { return nil }
func (modeTestAuthStore) StopWatch()                                          {}

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
		"DEPLOY=",
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
}

func TestNewCommandFlagSet_ExposesOnlyConfigFlag(t *testing.T) {
	fs := newCommandFlagSet("cockpit")
	if fs.Lookup("config") == nil {
		t.Fatal("expected -config flag to exist")
	}
	flagNames := make([]string, 0)
	fs.VisitAll(func(f *flag.Flag) {
		flagNames = append(flagNames, f.Name)
	})
	if len(flagNames) != 1 || flagNames[0] != "config" {
		t.Fatalf("expected only -config flag, got %v", flagNames)
	}
}

func TestNewCommandFlagSet_ParsesConfigOverride(t *testing.T) {
	fs := newCommandFlagSet("cockpit")
	if err := fs.Parse([]string{"-config", "/tmp/custom-config.yaml"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := fs.Lookup("config"); got == nil {
		t.Fatal("expected -config flag lookup after parse")
	} else if got.Value.String() != "/tmp/custom-config.yaml" {
		t.Fatalf("expected config flag value to be propagated, got %q", got.Value.String())
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

func TestResolveBootstrapConfig_UsesNacosBeforeStatic(t *testing.T) {
	nacosSource := modeTestConfigSource{mode: "nacos"}
	nacosAuth := modeTestAuthStore{}
	staticCalls := 0

	result, err := resolveBootstrapConfig("/tmp/config.yaml", bootstrapLoaders{
		nacosAddr: "127.0.0.1:8848",
		loadNacos: func() (*bootstrapConfig, error) {
			return &bootstrapConfig{
				cfg:          &config.Config{Port: 8080},
				configSource: nacosSource,
				authStore:    nacosAuth,
			}, nil
		},
		loadStatic: func(string) (*bootstrapConfig, error) {
			staticCalls++
			return &bootstrapConfig{cfg: &config.Config{Port: 9090}, configSource: modeTestConfigSource{mode: "static"}}, nil
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
	if staticCalls != 0 {
		t.Fatalf("expected static config not to be loaded, got %d call(s)", staticCalls)
	}
}

func TestResolveBootstrapConfig_FallsBackToStaticWhenNacosFails(t *testing.T) {
	result, err := resolveBootstrapConfig("/tmp/config.yaml", bootstrapLoaders{
		nacosAddr: "127.0.0.1:8848",
		loadNacos: func() (*bootstrapConfig, error) {
			return nil, errors.New("nacos unavailable")
		},
		loadStatic: func(path string) (*bootstrapConfig, error) {
			if path != "/tmp/config.yaml" {
				t.Fatalf("expected static config path to be forwarded, got %q", path)
			}
			return &bootstrapConfig{
				cfg:          &config.Config{Port: 9090},
				configSource: modeTestConfigSource{mode: "static"},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("resolveBootstrapConfig() error = %v", err)
	}
	if result == nil {
		t.Fatal("resolveBootstrapConfig() returned nil result")
	}
	if result.configSource.Mode() != "static" {
		t.Fatalf("expected static config source after nacos failure, got %q", result.configSource.Mode())
	}
	if result.authStore != nil {
		t.Fatal("expected static fallback to defer auth store construction")
	}
}

func TestResolveBootstrapConfig_ReturnsErrorWhenAllSourcesFail(t *testing.T) {
	_, err := resolveBootstrapConfig("/tmp/config.yaml", bootstrapLoaders{
		nacosAddr: "127.0.0.1:8848",
		loadNacos: func() (*bootstrapConfig, error) {
			return nil, errors.New("nacos unavailable")
		},
		loadStatic: func(string) (*bootstrapConfig, error) {
			return nil, errors.New("config file missing")
		},
	})
	if err == nil {
		t.Fatal("expected error when both nacos and static config fail")
	}
	if !strings.Contains(err.Error(), "nacos") || !strings.Contains(err.Error(), "config file missing") {
		t.Fatalf("expected combined bootstrap error, got %q", err)
	}
}

func TestResolveBootstrapConfig_UsesStaticWhenNacosIsUnconfigured(t *testing.T) {
	nacosCalls := 0
	result, err := resolveBootstrapConfig("/tmp/config.yaml", bootstrapLoaders{
		loadNacos: func() (*bootstrapConfig, error) {
			nacosCalls++
			return nil, nil
		},
		loadStatic: func(string) (*bootstrapConfig, error) {
			return &bootstrapConfig{
				cfg:          &config.Config{Port: 9090},
				configSource: modeTestConfigSource{mode: "static"},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("resolveBootstrapConfig() error = %v", err)
	}
	if nacosCalls != 0 {
		t.Fatalf("expected nacos loader to be skipped when unconfigured, got %d call(s)", nacosCalls)
	}
	if result == nil || result.configSource == nil || result.configSource.Mode() != "static" {
		t.Fatalf("expected static config source, got %#v", result)
	}
}

func TestWarnIfUsingLocalStaticMode_LogsInfo(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "local static mode"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			previousOutput := log.StandardLogger().Out
			previousFormatter := log.StandardLogger().Formatter
			previousLevel := log.GetLevel()
			log.SetOutput(&buffer)
			log.SetFormatter(&log.TextFormatter{DisableTimestamp: true, DisableColors: true})
			log.SetLevel(log.InfoLevel)
			t.Cleanup(func() {
				log.SetOutput(previousOutput)
				log.SetFormatter(previousFormatter)
				log.SetLevel(previousLevel)
			})

			warnIfUsingLocalStaticMode(nacos.NewStaticConfigSource("/tmp/config.yaml"), "/tmp/config.yaml")

			output := buffer.String()
			if !strings.Contains(output, "local static file mode") {
				t.Fatalf("expected static-mode warning log, got %q", output)
			}
			if !strings.Contains(output, "/tmp/config.yaml") {
				t.Fatalf("expected warning to mention config path, got %q", output)
			}
			if !strings.Contains(output, "NACOS_ADDR") {
				t.Fatalf("expected info log to point users to NACOS_ADDR, got %q", output)
			}
			if !strings.Contains(output, "level=info") {
				t.Fatalf("expected info log level, got %q", output)
			}
		})
	}
}

func TestWarnIfUsingLocalStaticMode_SkipsNonLocalStaticModes(t *testing.T) {
	tests := []struct {
		name         string
		configSource nacos.ConfigSource
	}{
		{
			name:         "nacos mode",
			configSource: modeTestConfigSource{mode: "nacos"},
		},
		{
			name:         "nil config source",
			configSource: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			previousOutput := log.StandardLogger().Out
			previousFormatter := log.StandardLogger().Formatter
			previousLevel := log.GetLevel()
			log.SetOutput(&buffer)
			log.SetFormatter(&log.TextFormatter{DisableTimestamp: true, DisableColors: true})
			log.SetLevel(log.InfoLevel)
			t.Cleanup(func() {
				log.SetOutput(previousOutput)
				log.SetFormatter(previousFormatter)
				log.SetLevel(previousLevel)
			})

			warnIfUsingLocalStaticMode(tc.configSource, "/tmp/config.yaml")

			if got := buffer.String(); got != "" {
				t.Fatalf("expected no static-mode log, got %q", got)
			}
		})
	}
}
