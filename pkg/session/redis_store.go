package session

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/vearne/agentscope-go/pkg/memory"
	asSession "github.com/vearne/agentscope-go/pkg/session"
)

// RedisStore persists sessions in Redis.
type RedisStore struct {
	mu     sync.Mutex
	redis  *redis.Client
	addr   string
	passwd string
	db     int
}

func NewRedisStore(addr, password string, db int) *RedisStore {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &RedisStore{
		redis:  rdb,
		addr:   addr,
		passwd: password,
		db:     db,
	}
}

func (s *RedisStore) currentKey(chatID string) string {
	val, err := s.redis.Get(context.Background(), "session_key:"+chatID).Result()
	if err == nil {
		return val
	}
	return chatID
}

func (s *RedisStore) setCurrentKey(ctx context.Context, chatID, key string) {
	s.redis.Set(ctx, "session_key:"+chatID, key, 0)
}

func (s *RedisStore) newSession(chatID string) (asSession.SessionBase, error) {
	key := s.currentKey(chatID)
	return asSession.NewRedisSession(s.addr, "session:"+key, asSession.WithRedisPassword(s.passwd), asSession.WithRedisDB(s.db)), nil
}

func (s *RedisStore) LoadSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	sess, err := s.newSession(chatID)
	if err != nil {
		return err
	}
	err = sess.Load(ctx, mem)
	if err != nil {
		if err.Error() == "get from redis: redis: nil" {
			return nil
		}
		return err
	}
	return nil
}

func (s *RedisStore) SaveSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	sess, err := s.newSession(chatID)
	if err != nil {
		return err
	}
	return sess.Save(ctx, mem)
}

func (s *RedisStore) NewSession(ctx context.Context, chatID string) string {
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

func (s *RedisStore) ResetSession(ctx context.Context, chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCurrentKey(ctx, chatID, chatID)
}
