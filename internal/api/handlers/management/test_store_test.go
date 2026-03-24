package management

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

type memoryAuthStore struct {
	mu    sync.Mutex
	items map[string]*coreauth.Auth
}

func (s *memoryAuthStore) List(_ context.Context) ([]*coreauth.Auth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*coreauth.Auth, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	return out, nil
}

func (s *memoryAuthStore) Save(_ context.Context, auth *coreauth.Auth) (string, error) {
	if auth == nil {
		return "", nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.items == nil {
		s.items = make(map[string]*coreauth.Auth)
	}
	s.items[auth.ID] = auth
	return auth.ID, nil
}

func (s *memoryAuthStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.items, id)
	return nil
}

func (s *memoryAuthStore) ReadByName(_ context.Context, name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item, ok := s.items[name]; ok && item != nil {
		return json.Marshal(item.Metadata)
	}
	return nil, nacos.ErrStaticMode

}

func (s *memoryAuthStore) ListMetadata(_ context.Context) ([]nacos.AuthFileMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]nacos.AuthFileMetadata, 0, len(s.items))
	for _, item := range s.items {
		if item == nil {
			continue
		}
		out = append(out, nacos.AuthFileMetadata{ID: item.ID, Name: item.FileName, Type: item.Provider, Email: item.Label})
	}
	return out, nil
}

func (s *memoryAuthStore) Watch(_ context.Context, _ func([]*coreauth.Auth)) error { return nil }

func (s *memoryAuthStore) StopWatch() {}

func (s *memoryAuthStore) SetBaseDir(string) {}

type recordingAuthStore struct {
	mu        sync.Mutex
	items     map[string]*coreauth.Auth
	saved     []*coreauth.Auth
	deleted   []string
	saveErr   error
	deleteErr error
}

func (s *recordingAuthStore) List(_ context.Context) ([]*coreauth.Auth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*coreauth.Auth, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item.Clone())
	}
	return out, nil
}

func (s *recordingAuthStore) Save(_ context.Context, auth *coreauth.Auth) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if auth == nil {
		return "", nil
	}
	clone := auth.Clone()
	s.saved = append(s.saved, clone)
	if s.saveErr != nil {
		return "", s.saveErr
	}
	if s.items == nil {
		s.items = make(map[string]*coreauth.Auth)
	}
	s.items[clone.ID] = clone
	return clone.ID, nil
}

func (s *recordingAuthStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.deleted = append(s.deleted, id)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.items, id)
	return nil
}

func (s *recordingAuthStore) ReadByName(_ context.Context, name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item == nil {
			continue
		}
		if item.FileName == name || item.ID == name {
			return json.Marshal(item.Metadata)
		}
	}
	return nil, nacos.ErrStaticMode
}

func (s *recordingAuthStore) ListMetadata(_ context.Context) ([]nacos.AuthFileMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]nacos.AuthFileMetadata, 0, len(s.items))
	for _, item := range s.items {
		if item == nil {
			continue
		}
		name := item.FileName
		if name == "" {
			name = item.ID
		}
		meta := nacos.AuthFileMetadata{ID: item.ID, Name: name, Type: item.Provider, Email: item.Label}
		if note, ok := item.Metadata["note"].(string); ok {
			meta.Note = note
		}
		if rawPriority, ok := item.Metadata["priority"]; ok {
			switch v := rawPriority.(type) {
			case int:
				pv := v
				meta.Priority = &pv
			case float64:
				pv := int(v)
				meta.Priority = &pv
			}
		}
		out = append(out, meta)
	}
	return out, nil
}

func (s *recordingAuthStore) Watch(_ context.Context, _ func([]*coreauth.Auth)) error { return nil }

func (s *recordingAuthStore) StopWatch() {}

func (s *recordingAuthStore) lastSaved() *coreauth.Auth {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.saved) == 0 {
		return nil
	}
	return s.saved[len(s.saved)-1].Clone()
}

type readonlyAuthStore struct{}

func (s *readonlyAuthStore) List(context.Context) ([]*coreauth.Auth, error) { return nil, nil }

func (s *readonlyAuthStore) Save(context.Context, *coreauth.Auth) (string, error) {
	return "", nacos.ErrStaticMode
}

func (s *readonlyAuthStore) Delete(context.Context, string) error { return nacos.ErrStaticMode }

func (s *readonlyAuthStore) ReadByName(context.Context, string) ([]byte, error) {
	return nil, nacos.ErrStaticMode
}

func (s *readonlyAuthStore) ListMetadata(context.Context) ([]nacos.AuthFileMetadata, error) {
	return nil, nacos.ErrStaticMode
}

func (s *readonlyAuthStore) Watch(context.Context, func([]*coreauth.Auth)) error { return nil }

func (s *readonlyAuthStore) StopWatch() {}
