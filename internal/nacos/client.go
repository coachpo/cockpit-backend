package nacos

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	log "github.com/sirupsen/logrus"
)

type Client struct {
	configClient config_client.IConfigClient
	group        string
}

// NewClientFromEnv creates a Nacos client from environment variables.
// Env vars: NACOS_ADDR (host:port), NACOS_NAMESPACE (default "public"),
// NACOS_USERNAME, NACOS_PASSWORD, NACOS_GROUP (default "DEFAULT_GROUP"),
// NACOS_CACHE_DIR (default "/tmp/nacos/cache")
func NewClientFromEnv() (*Client, error) {
	addr := os.Getenv("NACOS_ADDR")
	if addr == "" {
		return nil, fmt.Errorf("NACOS_ADDR is required")
	}

	host, portStr, err := parseHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.ParseUint(portStr, 10, 64)
	if port == 0 {
		port = 8848
	}

	namespace := os.Getenv("NACOS_NAMESPACE")
	if namespace == "" {
		namespace = "public"
	}
	username := os.Getenv("NACOS_USERNAME")
	password := os.Getenv("NACOS_PASSWORD")
	group := os.Getenv("NACOS_GROUP")
	if group == "" {
		group = "DEFAULT_GROUP"
	}
	cacheDir := os.Getenv("NACOS_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "/tmp/nacos/cache"
	}

	sc := []constant.ServerConfig{{IpAddr: host, Port: port}}
	cc := constant.ClientConfig{
		NamespaceId:         namespace,
		TimeoutMs:           5000,
		DisableUseSnapShot:  true,
		NotLoadCacheAtStart: true,
		CacheDir:            cacheDir,
		LogDir:              "/tmp/nacos/log",
		LogLevel:            "warn",
		Username:            username,
		Password:            password,
	}

	configClient, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig:  &cc,
		ServerConfigs: sc,
	})
	if err != nil {
		return nil, fmt.Errorf("nacos client init: %w", err)
	}

	log.Infof("nacos client initialized for %s (namespace=%s, group=%s)", addr, namespace, group)
	return &Client{configClient: configClient, group: group}, nil
}

func (c *Client) ConfigClient() config_client.IConfigClient { return c.configClient }

func (c *Client) Group() string { return c.group }

func (c *Client) AvailabilityError() error {
	if c == nil || c.configClient == nil {
		return fmt.Errorf("nacos client is nil")
	}
	_, err := c.configClient.GetConfig(vo.ConfigParam{
		DataId: "__health_check__",
		Group:  c.group,
	})
	return err
}

// IsAvailable checks if Nacos is reachable by attempting a GetConfig.
func (c *Client) IsAvailable() bool {
	// GetConfig returns error only on connection failure, not on missing config.
	return c.AvailabilityError() == nil
}

func (c *Client) WaitUntilAvailable(timeout, interval time.Duration) error {
	if c == nil || c.configClient == nil {
		return fmt.Errorf("nacos client is nil")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}

	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		lastErr = c.AvailabilityError()
		if lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("nacos client did not become ready within %s: %w", timeout, lastErr)
}

func parseHostPort(addr string) (string, string, error) {
	normalized, err := normalizeNacosAddr(addr)
	if err != nil {
		return "", "", err
	}

	host := normalized
	port := "8848"

	if parsedHost, parsedPort, errSplit := net.SplitHostPort(normalized); errSplit == nil {
		host = parsedHost
		port = parsedPort
	} else {
		switch {
		case strings.Count(normalized, ":") == 1 && !strings.HasPrefix(normalized, "["):
			parts := strings.SplitN(normalized, ":", 2)
			host = parts[0]
			port = parts[1]
		case strings.HasPrefix(normalized, "[") && strings.HasSuffix(normalized, "]"):
			host = strings.TrimSuffix(strings.TrimPrefix(normalized, "["), "]")
		}
	}

	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if host == "" {
		return "", "", fmt.Errorf("NACOS_ADDR host is required")
	}

	return host, port, nil
}

func normalizeNacosAddr(addr string) (string, error) {
	normalized := strings.TrimSpace(addr)
	if normalized == "" {
		return "", fmt.Errorf("NACOS_ADDR is required")
	}
	if strings.Contains(normalized, "://") {
		parsed, err := url.Parse(normalized)
		if err != nil {
			return "", fmt.Errorf("parse NACOS_ADDR: %w", err)
		}
		normalized = strings.TrimSpace(parsed.Host)
	}
	normalized = strings.TrimSuffix(normalized, "/")
	if normalized == "" {
		return "", fmt.Errorf("NACOS_ADDR host is required")
	}
	return normalized, nil
}
