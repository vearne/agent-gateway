package channel

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

type ChannelManager struct {
	mu       sync.Mutex
	channels []*Channel
	cancel   context.CancelFunc
}

func NewChannelManager() *ChannelManager {
	return &ChannelManager{}
}

func (m *ChannelManager) Add(ch *Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = append(m.channels, ch)
}

func (m *ChannelManager) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Lock()
	channels := make([]*Channel, len(m.channels))
	copy(channels, m.channels)
	m.mu.Unlock()

	for _, ch := range channels {
		zap.L().Info("starting channel", zap.String("name", ch.name))
		ch.Start(ctx)
	}
}

func (m *ChannelManager) Stop() {
	m.mu.Lock()
	channels := make([]*Channel, len(m.channels))
	copy(channels, m.channels)
	m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}

	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func(c *Channel) {
			defer wg.Done()
			done := make(chan struct{})
			go func() {
				c.Stop()
				close(done)
			}()
			select {
			case <-done:
				zap.L().Info("channel stopped", zap.String("name", c.name))
			case <-time.After(10 * time.Second):
				zap.L().Warn("channel stop timed out", zap.String("name", c.name))
			}
		}(ch)
	}
	wg.Wait()
}
