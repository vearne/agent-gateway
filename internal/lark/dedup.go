package lark

import (
	"sync"
	"time"
)

const dedupTTL = 600 // seconds

type msgDedup struct {
	mu   sync.Mutex
	seen map[string]int64
}

func newMsgDedup() *msgDedup {
	return &msgDedup{seen: make(map[string]int64)}
}

func (d *msgDedup) tryAdd(msgID string) bool {
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

var globalDedup = newMsgDedup()
