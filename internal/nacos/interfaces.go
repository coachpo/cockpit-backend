package nacos

import (
	"context"
	"time"

	"github.com/coachpo/cockpit-backend/internal/config"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
)

type AuthFileMetadata struct {
	ID       string
	Name     string
	Type     string
	Email    string
	Priority *int
	Note     string
	Size     int64
	ModTime  time.Time
}

type ConfigSource interface {
	LoadConfig() (*config.Config, error)
	SaveConfig(cfg *config.Config) error
	WatchConfig(onChange func(*config.Config)) error
	StopWatch()
	Mode() string
}

type WatchableAuthStore interface {
	coreauth.Store
	ReadByName(ctx context.Context, name string) ([]byte, error)
	ListMetadata(ctx context.Context) ([]AuthFileMetadata, error)
	Watch(ctx context.Context, onChange func([]*coreauth.Auth)) error
	StopWatch()
}
