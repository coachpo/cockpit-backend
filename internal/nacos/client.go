package nacos

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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
// Returns nil if NACOS_ADDR is not set.
// Env vars: NACOS_ADDR (host:port), NACOS_NAMESPACE (default "public"),
// NACOS_USERNAME, NACOS_PASSWORD, NACOS_GROUP (default "DEFAULT_GROUP"),
// NACOS_CACHE_DIR (default "/tmp/nacos/cache")
func NewClientFromEnv() (*Client, error) {
	addr := os.Getenv("NACOS_ADDR")
	if addr == "" {
		return nil, nil // Nacos not configured
	}

	host, portStr := parseHostPort(addr)
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

	log.Infof("nacos client connected to %s (namespace=%s, group=%s)", addr, namespace, group)
	return &Client{configClient: configClient, group: group}, nil
}

func (c *Client) ConfigClient() config_client.IConfigClient { return c.configClient }

func (c *Client) Group() string { return c.group }

// IsAvailable checks if Nacos is reachable by attempting a GetConfig.
func (c *Client) IsAvailable() bool {
	_, err := c.configClient.GetConfig(vo.ConfigParam{
		DataId: "__health_check__",
		Group:  c.group,
	})
	// GetConfig returns error only on connection failure, not on missing config
	return err == nil
}

func parseHostPort(addr string) (string, string) {
	// handle host:port
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return addr, "8848"
}
