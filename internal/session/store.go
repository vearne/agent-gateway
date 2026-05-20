package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/vearne/agentscope-go/pkg/memory"
	asSession "github.com/vearne/agentscope-go/pkg/session"
)

type Store struct {
	mu     sync.Mutex
	redis  *redis.Client
	dir    string
	addr   string
	passwd string
	db     int
	keyMap map[string]string
}

func NewFileStore(dir string) *Store {
	return &Store{
		dir:    dir,
		keyMap: make(map[string]string),
	}
}

func NewRedisStore(addr, password string, db int) *Store {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &Store{
		redis:  rdb,
		addr:   addr,
		passwd: password,
		db:     db,
		keyMap: make(map[string]string),
	}
}

func (s *Store) currentKey(chatID string) string {
	if s.redis != nil {
		val, err := s.redis.Get(context.Background(), "session_key:"+chatID).Result()
		if err == nil {
			return val
		}
		return chatID
	}
	if k, ok := s.keyMap[chatID]; ok {
		return k
	}
	return chatID
}

func (s *Store) setCurrentKey(ctx context.Context, chatID, key string) {
	if s.redis != nil {
		s.redis.Set(ctx, "session_key:"+chatID, key, 0)
		return
	}
	s.keyMap[chatID] = key
}

func (s *Store) newSession(chatID string) (asSession.SessionBase, error) {
	key := s.currentKey(chatID)
	if s.redis != nil {
		return asSession.NewRedisSession(s.addr, "session:"+key, asSession.WithRedisPassword(s.passwd), asSession.WithRedisDB(s.db)), nil
	}
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	fp := filepath.Join(s.dir, key+".json")
	return asSession.NewJSONSession(fp), nil
}

func (s *Store) LoadSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	sess, err := s.newSession(chatID)
	if err != nil {
		return err
	}
	err = sess.Load(ctx, mem)
	if err != nil {
		if s.redis != nil {
			if err.Error() == "get from redis: redis: nil" {
				return nil
			}
		} else {
			if os.IsNotExist(unwrapPathError(err)) {
				return nil
			}
		}
		return err
	}
	return nil
}

func (s *Store) SaveSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	sess, err := s.newSession(chatID)
	if err != nil {
		return err
	}
	return sess.Save(ctx, mem)
}

func (s *Store) NewSession(ctx context.Context, chatID string) string {
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

func (s *Store) ResetSession(ctx context.Context, chatID string) {
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
