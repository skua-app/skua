package events

import (
	"container/list"
	"sync"
	"time"
)

// clipCacheMaxEntries caps the number of distinct event clips held in memory.
const clipCacheMaxEntries = 16

// clipCacheMaxBytes caps the total bytes held across all cached clips.
// 16 × 32 MiB ≈ 512 MiB worst case; in practice a Frigate 30s clip at
// 3500kbps is ~13 MiB, so a fully populated cache is closer to 200 MiB.
const clipCacheMaxBytes int64 = 512 << 20

// clipEntry is the cached payload for one event id. data is treated as
// immutable after Put; concurrent readers share the same backing array via
// bytes.NewReader, which is safe.
type clipEntry struct {
	id      string
	data    []byte
	modTime time.Time
}

// clipCache is a process-lifetime LRU keyed by Frigate event id. Event ids
// are immutable in Frigate once finalised (clips are write-once), so no TTL
// is needed — eviction is purely by recency under entry-count and byte caps.
//
// Concurrency: a single sync.Mutex guards both the map and the list. All
// public methods take the lock; the cached []byte values are returned to
// callers by reference and must not be mutated.
type clipCache struct {
	mu         sync.Mutex
	maxEntries int
	maxBytes   int64
	curBytes   int64
	index      map[string]*list.Element // value: *clipEntry
	order      *list.List               // MRU at front, LRU at back
}

// newClipCache returns a cache with the given caps. Both caps are enforced
// on Put; entries larger than maxBytes are not cached.
func newClipCache(maxEntries int, maxBytes int64) *clipCache {
	return &clipCache{
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		index:      make(map[string]*list.Element),
		order:      list.New(),
	}
}

// Get returns the cached bytes and modTime for id if present, moving the
// entry to MRU. The returned slice must be treated as immutable.
func (c *clipCache) Get(id string) ([]byte, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.index[id]
	if !ok {
		return nil, time.Time{}, false
	}
	c.order.MoveToFront(el)
	e := el.Value.(*clipEntry)
	return e.data, e.modTime, true
}

// Put inserts data under id, evicting LRU entries as needed to stay within
// both caps. Returns false if the single entry would not fit even in an
// otherwise empty cache; in that case nothing is stored.
func (c *clipCache) Put(id string, data []byte, modTime time.Time) bool {
	size := int64(len(data))
	c.mu.Lock()
	defer c.mu.Unlock()

	if size > c.maxBytes {
		return false
	}

	if el, ok := c.index[id]; ok {
		old := el.Value.(*clipEntry)
		c.curBytes -= int64(len(old.data))
		old.data = data
		old.modTime = modTime
		c.curBytes += size
		c.order.MoveToFront(el)
		c.evictLocked()
		return true
	}

	entry := &clipEntry{id: id, data: data, modTime: modTime}
	el := c.order.PushFront(entry)
	c.index[id] = el
	c.curBytes += size
	c.evictLocked()
	return true
}

// evictLocked drops LRU entries until both caps are satisfied. Caller must
// hold c.mu.
func (c *clipCache) evictLocked() {
	for c.order.Len() > c.maxEntries || c.curBytes > c.maxBytes {
		back := c.order.Back()
		if back == nil {
			return
		}
		e := back.Value.(*clipEntry)
		c.order.Remove(back)
		delete(c.index, e.id)
		c.curBytes -= int64(len(e.data))
	}
}

// len returns the number of cached entries (test helper).
func (c *clipCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
