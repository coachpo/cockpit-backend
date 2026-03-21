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

	configaccess "github.com/coachpo/cockpit-backend/internal/access/config_access"
	"github.com/coachpo/cockpit-backend/internal/buildinfo"
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

var (
	Version           = "dev"
	Commit            = "none"
	BuildDate         = "unknown"
	DefaultConfigPath = ""
)

// init initializes the shared logger setup.
func init() {
	logging.SetupBaseLogger()
	buildinfo.Version = Version
	buildinfo.Commit = Commit
	buildinfo.BuildDate = BuildDate
}

func warnIfUsingLocalStaticMode(configSource nacos.ConfigSource, configFilePath string) {
	if configSource == nil || configSource.Mode() != "static" {
		return
	}
	log.Warnf("Running in local static file mode using %q; local config file changes are not watched. Set NACOS_ADDR to enable live config reloads.", configFilePath)
}

// main is the entry point of the application.
// It parses command-line flags, loads configuration, and starts the appropriate
// service based on the provided flags (login, codex-login, or server mode).
func main() {
	fmt.Printf("Cockpit Version: %s, Commit: %s, BuiltAt: %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate)

	// Command-line flags to control the application's behavior.
	var configPath string

	// Define command-line flags for different operation modes.
	flag.StringVar(&configPath, "config", DefaultConfigPath, "Static config file path (ignored when NACOS_ADDR is set)")

	flag.CommandLine.Usage = func() {
		out := flag.CommandLine.Output()
		_, _ = fmt.Fprintf(out, "Usage of %s\n", os.Args[0])
		flag.CommandLine.VisitAll(func(f *flag.Flag) {
			s := fmt.Sprintf("  -%s", f.Name)
			name, unquoteUsage := flag.UnquoteUsage(f)
			if name != "" {
				s += " " + name
			}
			if len(s) <= 4 {
				s += "	"
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
	}

	// Parse the command-line flags.
	flag.Parse()

	// Core application variables.
	var err error
	var cfg *config.Config
	var isCloudDeploy bool

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

	// Check for cloud deploy mode only on first execution
	// Read env var name in uppercase: DEPLOY
	deployEnv := os.Getenv("DEPLOY")
	if deployEnv == "cloud" {
		isCloudDeploy = true
	}

	// Determine and load the configuration file.
	var configFilePath string
	if configPath != "" {
		configFilePath = configPath
	} else {
		wd, err = os.Getwd()
		if err != nil {
			log.Errorf("failed to get working directory: %v", err)
			return
		}
		configFilePath = filepath.Join(wd, "config.yaml")
	}

	var configSource nacos.ConfigSource
	var authStore nacos.WatchableAuthStore
	if os.Getenv("NACOS_ADDR") != "" {
		client, errNewClient := nacos.NewClientFromEnv()
		if errNewClient != nil {
			log.Errorf("failed to create nacos client: %v", errNewClient)
			return
		}
		if client == nil {
			log.Error("nacos configured but client was not created")
			return
		}
		configSource = nacos.NewNacosConfigStore(client)
		authStore = nacos.NewNacosAuthStore(client)
		cfg, err = configSource.LoadConfig()
		if err != nil {
			log.Errorf("failed to load config from nacos: %v", err)
			return
		}
	} else {
		cfg, err = config.LoadConfigOptional(configFilePath, isCloudDeploy)
		if err != nil {
			log.Errorf("failed to load config: %v", err)
			return
		}
		configSource = nacos.NewStaticConfigSource(configFilePath)
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	// In cloud deploy mode, check if we have a valid configuration
	var configFileExists bool
	if isCloudDeploy {
		if configSource != nil && configSource.Mode() == "nacos" {
			if cfg.Port == 0 {
				log.Info("Cloud deploy mode: Nacos configuration is empty or invalid; standing by for valid configuration")
				configFileExists = false
			} else {
				log.Info("Cloud deploy mode: Nacos configuration detected; starting service")
				configFileExists = true
			}
		} else {
			if info, errStat := os.Stat(configFilePath); errStat != nil {
				log.Info("Cloud deploy mode: No configuration file detected; standing by for configuration")
				configFileExists = false
			} else if info.IsDir() {
				log.Info("Cloud deploy mode: Config path is a directory; standing by for configuration")
				configFileExists = false
			} else if cfg.Port == 0 {
				log.Info("Cloud deploy mode: Configuration file is empty or invalid; standing by for valid configuration")
				configFileExists = false
			} else {
				log.Info("Cloud deploy mode: Configuration file detected; starting service")
				configFileExists = true
			}
		}
	}
	coreauth.SetQuotaCooldownDisabled(cfg.DisableCooling)

	if err = logging.ConfigureLogOutput(cfg); err != nil {
		log.Errorf("failed to configure log output: %v", err)
		return
	}
	warnIfUsingLocalStaticMode(configSource, configFilePath)

	log.Infof("Cockpit Version: %s, Commit: %s, BuiltAt: %s", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate)

	// Set the log level based on the configuration.
	util.SetLogLevel(cfg)

	if resolvedAuthDir, errResolveAuthDir := util.ResolveAuthDir(cfg.AuthDir); errResolveAuthDir != nil {
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

	// In cloud deploy mode without config file, just wait for shutdown signals
	if isCloudDeploy && !configFileExists {
		// No config file available, just wait for shutdown
		cmd.WaitForCloudDeploy()
		return
	}

	// Start the main proxy service
	registry.StartModelsUpdater(context.Background())
	cmd.StartService(cfg, configFilePath, configSource, authStore)
}
