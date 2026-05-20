package lark

import (
	"sync"
	"time"
)

const dedupTTL = 600 // seconds

type MsgDedup struct {
	mu   sync.Mutex
	seen map[string]int64
}

func NewMsgDedup() *MsgDedup {
	return &MsgDedup{seen: make(map[string]int64)}
}

func (d *MsgDedup) TryAdd(msgID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().Unix()
	deadline := now - dedupTTL

	if ts, ok := d.seen[msgID]; ok && ts > deadline {
		return false
	}

	d.seen[msgID] = now

	if len(d.seen) > 5000 {
		for k, ts := range d.seen {
			if ts <= deadline {
				delete(d.seen, k)
			}
		}
	}

	return true
}

var globalDedup = NewMsgDedup()
