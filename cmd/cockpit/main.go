// Package main provides the entry point for the Cockpit server.
// This server acts as a Nacos-first OpenAI-compatible proxy for Cockpit runtime auth and routing.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	configaccess "github.com/coachpo/cockpit-backend/internal/access/config_access"
	"github.com/coachpo/cockpit-backend/internal/cmd"
	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/logging"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	"github.com/coachpo/cockpit-backend/internal/registry"
	_ "github.com/coachpo/cockpit-backend/internal/translator"
	"github.com/coachpo/cockpit-backend/internal/util"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
)

const configPathUsage = "Path to the YAML config file (defaults to ./cockpit/config.yaml)"

// init initializes the shared logger setup.
func init() {
	logging.SetupBaseLogger()
}

func warnIfUsingLocalStaticMode(configSource nacos.ConfigSource, configFilePath string) {
	if configSource == nil || configSource.Mode() != "static" {
		return
	}
	log.Infof("Running in local static file mode using %q; local config file changes are not watched. Set NACOS_ADDR to enable live config reloads.", configFilePath)
}

func newCommandFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.String("config", "", configPathUsage)
	fs.Usage = func() {
		out := fs.Output()
		_, _ = fmt.Fprintf(out, "Usage of %s\n", name)
		hasFlags := false
		fs.VisitAll(func(f *flag.Flag) {
			hasFlags = true
			s := fmt.Sprintf("  -%s", f.Name)
			usageName, unquoteUsage := flag.UnquoteUsage(f)
			if usageName != "" {
				s += " " + usageName
			}
			if len(s) <= 4 {
				s += "\t"
			} else {
				s += "\n    "
			}
			if unquoteUsage != "" {
				s += unquoteUsage
			}
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
				s += fmt.Sprintf(" (default %s)", f.DefValue)
			}
			_, _ = fmt.Fprint(out, s+"\n")
		})
		if !hasFlags {
			_, _ = fmt.Fprintln(out, "  (no command-line flags)")
		}
	}
	return fs
}

func parseCommandArgs(name string, args []string) (string, error) {
	fs := newCommandFlagSet(name)
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() > 0 {
		return "", fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if configPathFlag := fs.Lookup("config"); configPathFlag != nil {
		return strings.TrimSpace(configPathFlag.Value.String()), nil
	}
	return "", nil
}

type bootstrapConfig struct {
	cfg          *config.Config
	configSource nacos.ConfigSource
	authStore    nacos.WatchableAuthStore
}

type bootstrapLoaders struct {
	nacosAddr  string
	loadNacos  func() (*bootstrapConfig, error)
	loadStatic func(configFilePath string) (*bootstrapConfig, error)
}

func resolveBootstrapConfig(configFilePath string, loaders bootstrapLoaders) (*bootstrapConfig, error) {
	if loaders.loadNacos == nil {
		loaders.loadNacos = loadNacosBootstrapConfig
	}
	if loaders.loadStatic == nil {
		loaders.loadStatic = loadStaticBootstrapConfig
	}

	nacosAddr := strings.TrimSpace(loaders.nacosAddr)
	if nacosAddr != "" {
		loaded, err := loaders.loadNacos()
		if err != nil {
			return nil, fmt.Errorf("failed to bootstrap from nacos: %w", err)
		}
		if err := validateBootstrapConfig("nacos", loaded); err != nil {
			return nil, fmt.Errorf("failed to bootstrap from nacos: %w", err)
		}
		return loaded, nil
	}

	loaded, err := loaders.loadStatic(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to bootstrap from local static config: %w", err)
	}
	if err := validateBootstrapConfig("static", loaded); err != nil {
		return nil, fmt.Errorf("failed to bootstrap from local static config: %w", err)
	}
	return loaded, nil
}

func loadNacosBootstrapConfig() (*bootstrapConfig, error) {
	client, err := nacos.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("create nacos client: %w", err)
	}
	if client == nil {
		return nil, fmt.Errorf("nacos configured but client was not created")
	}

	configSource := nacos.NewNacosConfigStore(client)
	authStore := nacos.NewNacosAuthStore(client)
	cfg, err := configSource.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config from nacos: %w", err)
	}

	return &bootstrapConfig{
		cfg:          cfg,
		configSource: configSource,
		authStore:    authStore,
	}, nil
}

func loadStaticBootstrapConfig(configFilePath string) (*bootstrapConfig, error) {
	cfg, err := config.LoadConfig(configFilePath)
	if err != nil {
		return nil, err
	}

	return &bootstrapConfig{
		cfg:          cfg,
		configSource: nacos.NewStaticConfigSource(configFilePath),
	}, nil
}

func validateBootstrapConfig(sourceName string, loaded *bootstrapConfig) error {
	if loaded == nil {
		return fmt.Errorf("%s bootstrap returned nil result", sourceName)
	}
	if loaded.configSource == nil {
		return fmt.Errorf("%s bootstrap returned nil config source", sourceName)
	}
	if loaded.cfg == nil {
		loaded.cfg = &config.Config{}
	}
	return nil
}

// main is the entry point of the application.
// It parses command-line flags, loads configuration, and starts the appropriate
// service based on the provided flags (login, codex-login, or server mode).
func main() {
	configFilePath, parseErr := parseCommandArgs(os.Args[0], os.Args[1:])
	if parseErr != nil {
		if errors.Is(parseErr, flag.ErrHelp) {
			return
		}
		log.Errorf("failed to parse command line flags: %v", parseErr)
		os.Exit(2)
	}

	// Core application variables.
	var err error

	wd, err := os.Getwd()
	if err != nil {
		log.Errorf("failed to get working directory: %v", err)
		return
	}

	// Load environment variables from .env if present.
	if errLoad := godotenv.Load(filepath.Join(wd, ".env")); errLoad != nil {
		if !errors.Is(errLoad, os.ErrNotExist) {
			log.WithError(errLoad).Warn("failed to load .env file")
		}
	}

	// Determine and load the configuration file.
	if configFilePath == "" {
		configFilePath = filepath.Join(wd, "cockpit", "config.yaml")
	}

	loaded, err := resolveBootstrapConfig(configFilePath, bootstrapLoaders{nacosAddr: os.Getenv("NACOS_ADDR")})
	if err != nil {
		log.Errorf("failed to bootstrap configuration: %v", err)
		os.Exit(1)
	}

	cfg := loaded.cfg
	configSource := loaded.configSource
	authStore := loaded.authStore
	coreauth.SetQuotaCooldownDisabled(cfg.DisableCooling)

	if err = logging.ConfigureLogOutput(cfg); err != nil {
		log.Errorf("failed to configure log output: %v", err)
		return
	}
	warnIfUsingLocalStaticMode(configSource, configFilePath)

	// Set the log level based on the configuration.
	util.SetLogLevel(cfg)

	configMode := ""
	if configSource != nil {
		configMode = configSource.Mode()
	}
	if resolvedAuthDir, errResolveAuthDir := util.ResolveRuntimeAuthDir(cfg.AuthDir, configMode); errResolveAuthDir != nil {
		log.Errorf("failed to resolve auth directory: %v", errResolveAuthDir)
		return
	} else {
		cfg.AuthDir = resolvedAuthDir
	}
	if authStore == nil {
		authStore = nacos.NewStaticAuthStore(cfg.AuthDir)
	}

	// Register built-in access providers before constructing services.
	configaccess.Register(&cfg.SDKConfig)

	// Start the main proxy service
	registry.StartModelsUpdater(context.Background())
	cmd.StartService(cfg, configFilePath, configSource, authStore)
}
