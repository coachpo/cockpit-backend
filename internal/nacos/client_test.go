package nacos

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

type availabilityStubConfigClient struct {
	errSequence []error
	getCalls    int
}

func (s *availabilityStubConfigClient) GetConfig(vo.ConfigParam) (string, error) {
	if s.getCalls < len(s.errSequence) {
		err := s.errSequence[s.getCalls]
		s.getCalls++
		return "", err
	}
	s.getCalls++
	return "", nil
}

func (s *availabilityStubConfigClient) PublishConfig(vo.ConfigParam) (bool, error) { return true, nil }

func (s *availabilityStubConfigClient) DeleteConfig(vo.ConfigParam) (bool, error) { return true, nil }

func (s *availabilityStubConfigClient) ListenConfig(vo.ConfigParam) error { return nil }

func (s *availabilityStubConfigClient) CancelListenConfig(vo.ConfigParam) error { return nil }

func (s *availabilityStubConfigClient) SearchConfig(vo.SearchConfigParam) (*model.ConfigPage, error) {
	return nil, nil
}

func (s *availabilityStubConfigClient) CloseClient() {}

func TestParseHostPort_NormalizesSupportedAddressForms(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantHost string
		wantPort string
	}{
		{
			name:     "plain host port",
			addr:     "192.168.1.222:8848",
			wantHost: "192.168.1.222",
			wantPort: "8848",
		},
		{
			name:     "http url",
			addr:     "http://192.168.1.222:8848",
			wantHost: "192.168.1.222",
			wantPort: "8848",
		},
		{
			name:     "https url with slash",
			addr:     "https://nacos.example.internal:8848/",
			wantHost: "nacos.example.internal",
			wantPort: "8848",
		},
		{
			name:     "host only defaults port",
			addr:     "nacos.internal",
			wantHost: "nacos.internal",
			wantPort: "8848",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := parseHostPort(tt.addr)
			if err != nil {
				t.Fatalf("parseHostPort() error = %v", err)
			}
			if host != tt.wantHost || port != tt.wantPort {
				t.Fatalf("parseHostPort() = (%q, %q), want (%q, %q)", host, port, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestClientWaitUntilAvailable_RetriesUntilReady(t *testing.T) {
	transientErr := errors.New("client not connected, current status:STARTING")
	stub := &availabilityStubConfigClient{errSequence: []error{transientErr, nil}}
	client := &Client{configClient: stub, group: "DEFAULT_GROUP"}

	if err := client.WaitUntilAvailable(20*time.Millisecond, time.Millisecond); err != nil {
		t.Fatalf("WaitUntilAvailable() error = %v", err)
	}
	if stub.getCalls < 2 {
		t.Fatalf("expected WaitUntilAvailable() to retry, got %d calls", stub.getCalls)
	}
}

func TestClientWaitUntilAvailable_ReturnsTimeoutError(t *testing.T) {
	transientErr := errors.New("client not connected, current status:STARTING")
	stub := &availabilityStubConfigClient{errSequence: []error{transientErr, transientErr, transientErr, transientErr}}
	client := &Client{configClient: stub, group: "DEFAULT_GROUP"}

	err := client.WaitUntilAvailable(2*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("expected WaitUntilAvailable() to fail when client never becomes ready")
	}
	if !strings.Contains(err.Error(), "did not become ready") || !strings.Contains(err.Error(), transientErr.Error()) {
		t.Fatalf("expected timeout error to mention readiness and last error, got %v", err)
	}
}
