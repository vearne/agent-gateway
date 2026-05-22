package session

import (
	"context"

	"github.com/vearne/agentscope-go/pkg/memory"
)

type NoneStore struct {
}

func NewNoneStore() *NoneStore {
	return new(NoneStore)
}

func (s *NoneStore) NewSession(ctx context.Context, chatID string) string {
	return chatID
}

func (s *NoneStore) LoadSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	return nil
}
func (s *NoneStore) SaveSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	return nil
}
func (s *NoneStore) ResetSession(ctx context.Context, chatID string) {

}
