package weixin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type AccountData struct {
	Token   string `json:"token,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	UserID  string `json:"user_id,omitempty"`
}

type Store struct {
	mu            sync.RWMutex
	dir           string
	account       *AccountData
	syncBuf       string
	contextTokens map[string]string
}

func NewStore(dir string) *Store {
	s := &Store{
		dir:           dir,
		contextTokens: make(map[string]string),
	}
	s.load()
	return s
}

func (s *Store) ensureDir() error {
	return os.MkdirAll(s.dir, 0755)
}

func (s *Store) load() {
	s.account = s.loadJSON("account.json", &AccountData{}).(*AccountData)

	var buf string
	raw := s.loadJSON("sync_buf.json", &struct{ Buf string `json:"buf"` }{})
	if v, ok := raw.(*struct{ Buf string `json:"buf"` }); ok {
		buf = v.Buf
	}
	s.syncBuf = buf

	raw = s.loadJSON("context_tokens.json", &struct{}{})
	if b, err := json.Marshal(raw); err == nil {
		var m map[string]string
		if json.Unmarshal(b, &m) == nil {
			s.contextTokens = m
		}
	}
	if s.contextTokens == nil {
		s.contextTokens = make(map[string]string)
	}
}

func (s *Store) loadJSON(name string, target interface{}) interface{} {
	p := filepath.Join(s.dir, name)
	data, err := os.ReadFile(p)
	if err != nil {
		return target
	}
	if err := json.Unmarshal(data, target); err != nil {
		return target
	}
	return target
}

func (s *Store) saveJSON(name string, v interface{}) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, name), data, 0644)
}

func (s *Store) SaveAccount(data *AccountData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.account = data
	return s.saveJSON("account.json", data)
}

func (s *Store) GetAccount() *AccountData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.account
}

func (s *Store) IsConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.account != nil && s.account.Token != ""
}

func (s *Store) SaveSyncBuf(buf string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncBuf = buf
	return s.saveJSON("sync_buf.json", map[string]string{"buf": buf})
}

func (s *Store) GetSyncBuf() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.syncBuf
}

func (s *Store) SetContextToken(chatID, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contextTokens[chatID] = token
	s.saveJSON("context_tokens.json", s.contextTokens)
}

func (s *Store) GetContextToken(chatID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.contextTokens[chatID]
}
