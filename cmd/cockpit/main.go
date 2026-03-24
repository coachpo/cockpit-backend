// Package main provides the entry point for the Cockpit server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	configaccess "github.com/coachpo/cockpit-backend/internal/access/config_access"
	"github.com/coachpo/cockpit-backend/internal/cmd"
	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/logging"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	"github.com/coachpo/cockpit-backend/internal/registry"
	_ "github.com/coachpo/cockpit-backend/internal/translator"
	"github.com/coachpo/cockpit-backend/internal/util"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
	log "github.com/sirupsen/logrus"
)

const (
	hostUsage                       = "HTTP host override for the management server listener"
	portUsage                       = "HTTP port override for the management server listener"
	nacosBootstrapReadinessTimeout  = 5 * time.Second
	nacosBootstrapReadinessInterval = 250 * time.Millisecond
)

type commandOptions struct {
	host    string
	hostSet bool
	port    int
	portSet bool
}

// init initializes the shared logger setup.
func init() {
	logging.SetupBaseLogger()
}

func newCommandFlagSet(name string, options *commandOptions) *flag.FlagSet {
	if options == nil {
		options = &commandOptions{}
	}

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&options.host, "host", "", hostUsage)
	fs.IntVar(&options.port, "port", 0, portUsage)
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

func parseCommandArgs(name string, args []string) (commandOptions, error) {
	var options commandOptions
	fs := newCommandFlagSet(name, &options)
	if err := fs.Parse(args); err != nil {
		return commandOptions{}, err
	}
	if fs.NArg() > 0 {
		return commandOptions{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "host":
			options.hostSet = true
			options.host = strings.TrimSpace(options.host)
		case "port":
			options.portSet = true
		}
	})

	if err := validateCommandOptions(options); err != nil {
		return commandOptions{}, err
	}

	return options, nil
}

func validateCommandOptions(options commandOptions) error {
	if options.hostSet && options.host == "" {
		return fmt.Errorf("host flag requires a non-empty value")
	}
	if options.portSet && (options.port < 1 || options.port > 65535) {
		return fmt.Errorf("port flag must be between 1 and 65535")
	}

	return nil
}

func applyRuntimeOverrides(cfg *config.Config, options commandOptions) error {
	if cfg == nil {
		return fmt.Errorf("runtime config is required")
	}
	if err := validateCommandOptions(options); err != nil {
		return err
	}

	if options.hostSet {
		cfg.Host = options.host
	}
	if options.portSet {
		cfg.Port = options.port
	}

	return nil
}

type bootstrapConfig struct {
	cfg          *config.Config
	configSource *nacos.NacosConfigStore
	authStore    *nacos.NacosAuthStore
}

type bootstrapLoaders struct {
	nacosAddr string
	loadNacos func() (*bootstrapConfig, error)
}

type nacosBootstrapAvailabilityClient interface {
	WaitUntilAvailable(timeout, interval time.Duration) error
}

func resolveBootstrapConfig(loaders bootstrapLoaders) (*bootstrapConfig, error) {
	if loaders.loadNacos == nil {
		loaders.loadNacos = loadNacosBootstrapConfig
	}

	nacosAddr := strings.TrimSpace(loaders.nacosAddr)
	if nacosAddr == "" {
		return nil, fmt.Errorf("failed to bootstrap from nacos: NACOS_ADDR is required")
	}

	loaded, err := loaders.loadNacos()
	if err != nil {
		return nil, fmt.Errorf("failed to bootstrap from nacos: %w", err)
	}
	if err := validateNacosBootstrapConfig(loaded); err != nil {
		return nil, fmt.Errorf("failed to bootstrap from nacos: %w", err)
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
	if err := waitForNacosBootstrapReadiness(client); err != nil {
		return nil, err
	}

	configSource := nacos.NewNacosConfigStore(client)
	authStore := nacos.NewNacosAuthStore(client)

	return bootstrapFromNacosStores(configSource, authStore, configSource.LoadConfig, authStore.List)

}

func waitForNacosBootstrapReadiness(client nacosBootstrapAvailabilityClient) error {
	if client == nil {
		return fmt.Errorf("wait for nacos availability: client is nil")
	}
	if err := client.WaitUntilAvailable(nacosBootstrapReadinessTimeout, nacosBootstrapReadinessInterval); err != nil {
		return fmt.Errorf("wait for nacos availability: %w", err)
	}
	return nil
}

func bootstrapFromNacosStores(
	configSource *nacos.NacosConfigStore,
	authStore *nacos.NacosAuthStore,
	loadConfig func() (*config.Config, error),
	loadAuths func(context.Context) ([]*coreauth.Auth, error),
) (*bootstrapConfig, error) {
	if configSource == nil {
		return nil, fmt.Errorf("nacos bootstrap requires a config source")
	}
	if authStore == nil {
		return nil, fmt.Errorf("nacos bootstrap requires an auth store")
	}
	if loadConfig == nil {
		return nil, fmt.Errorf("nacos bootstrap requires a config loader")
	}
	if loadAuths == nil {
		return nil, fmt.Errorf("nacos bootstrap requires an auth loader")
	}

	cfg, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config from nacos: %w", err)
	}
	if _, err = loadAuths(context.Background()); err != nil {
		return nil, fmt.Errorf("load auths from nacos: %w", err)
	}

	return &bootstrapConfig{
		cfg:          cfg,
		configSource: configSource,
		authStore:    authStore,
	}, nil
}

func validateNacosBootstrapConfig(loaded *bootstrapConfig) error {
	if loaded == nil {
		return fmt.Errorf("nacos bootstrap returned nil result")
	}
	if loaded.configSource == nil {
		return fmt.Errorf("nacos bootstrap returned nil config source")
	}
	if loaded.authStore == nil {
		return fmt.Errorf("nacos bootstrap returned nil auth store")
	}
	if loaded.cfg == nil {
		return fmt.Errorf("nacos bootstrap returned nil config")
	}
	return nil
}

// main is the entry point of the application.
// It parses command-line flags, loads configuration, and starts the appropriate
// service based on the provided flags (login, codex-login, or server mode).
func main() {
	options, parseErr := parseCommandArgs(os.Args[0], os.Args[1:])
	if parseErr != nil {
		if errors.Is(parseErr, flag.ErrHelp) {
			return
		}
		log.Errorf("failed to parse command line flags: %v", parseErr)
		os.Exit(2)
	}

	// Core application variables.
	var err error

	loaded, err := resolveBootstrapConfig(bootstrapLoaders{nacosAddr: os.Getenv("NACOS_ADDR")})
	if err != nil {
		log.Errorf("failed to bootstrap configuration: %v", err)
		os.Exit(1)
	}

	cfg := loaded.cfg
	configSource := loaded.configSource
	authStore := loaded.authStore
	if err = applyRuntimeOverrides(cfg, options); err != nil {
		log.Errorf("failed to apply runtime flag overrides: %v", err)
		os.Exit(2)
	}
	coreauth.SetQuotaCooldownDisabled(cfg.DisableCooling)

	if err = logging.ConfigureLogOutput(cfg); err != nil {
		log.Errorf("failed to configure log output: %v", err)
		return
	}

	// Set the log level based on the configuration.
	util.SetLogLevel(cfg)

	// Register built-in access providers before constructing services.
	configaccess.Register(&cfg.SDKConfig)

	// Start the main proxy service
	registry.StartModelsUpdater(context.Background())
	cmd.StartService(cfg, configSource, authStore)
}
