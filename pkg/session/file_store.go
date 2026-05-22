package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/vearne/agentscope-go/pkg/memory"
	asSession "github.com/vearne/agentscope-go/pkg/session"
)

// FileStore persists sessions as JSON files on disk, organized by chatID subdirectory.
type FileStore struct {
	mu     sync.Mutex
	dir    string
	keyMap map[string]string
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	fs := &FileStore{
		dir:    dir,
		keyMap: make(map[string]string),
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read session dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		kp := filepath.Join(dir, e.Name(), "keymap.json")
		data, err := os.ReadFile(kp)
		if err != nil {
			continue
		}
		var m map[string]string
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		for k, v := range m {
			fs.keyMap[k] = v
		}
	}
	return fs, nil
}

func (s *FileStore) currentKey(chatID string) string {
	if k, ok := s.keyMap[chatID]; ok {
		return k
	}
	return chatID
}

func (s *FileStore) setCurrentKey(_ context.Context, chatID, key string) {
	s.keyMap[chatID] = key
	s.saveKeyMap(chatID)
}

func (s *FileStore) saveKeyMap(chatID string) {
	kp := filepath.Join(s.dir, chatID, "keymap.json")
	m := map[string]string{chatID: s.keyMap[chatID]}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(kp, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "session: save key map %s: %v\n", kp, err)
	}
}

func (s *FileStore) newSession(chatID string) (asSession.SessionBase, error) {
	key := s.currentKey(chatID)
	chatDir := filepath.Join(s.dir, chatID)
	if err := os.MkdirAll(chatDir, 0755); err != nil {
		return nil, fmt.Errorf("create chat session dir: %w", err)
	}
	fp := filepath.Join(chatDir, key+".json")
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		if err := os.WriteFile(fp, []byte("[]"), 0644); err != nil {
			return nil, fmt.Errorf("create session file: %w", err)
		}
	}
	return asSession.NewJSONSession(fp), nil
}

func (s *FileStore) LoadSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	sess, err := s.newSession(chatID)
	if err != nil {
		return err
	}
	err = sess.Load(ctx, mem)
	if err != nil {
		if os.IsNotExist(unwrapPathError(err)) {
			return nil
		}
		return err
	}
	return nil
}

func (s *FileStore) SaveSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	sess, err := s.newSession(chatID)
	if err != nil {
		return err
	}
	return sess.Save(ctx, mem)
}

func (s *FileStore) NewSession(ctx context.Context, chatID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.currentKey(chatID)
	base := chatID
	seq := 0

	if current != chatID {
		suffix := current[len(chatID)+1:]
		if n, err := strconv.Atoi(suffix); err == nil {
			seq = n
		}
	}

	seq++
	newKey := fmt.Sprintf("%s_%d", base, seq)
	s.setCurrentKey(ctx, chatID, newKey)
	return newKey
}

func (s *FileStore) ResetSession(ctx context.Context, chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCurrentKey(ctx, chatID, chatID)
}

func unwrapPathError(err error) error {
	if pe, ok := err.(*os.PathError); ok {
		return pe
	}
	return err
}
