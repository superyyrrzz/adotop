package ui

// diffBodyCache is an in-memory cache of rendered diff bodies keyed by
// the (sourceSha, targetSha, path) tuple from diffSelectionKey. Entries
// survive across PR re-opens so bouncing list ↔ detail doesn't refetch
// every diff.
//
// We track which PR each key belongs to so we can evict whole PRs FIFO
// once we exceed maxPRs. We also keep `inflight` slots (value=nil) so
// the prefetcher doesn't re-issue duplicate requests.
type diffBodyCache struct {
	bodies   map[string][]byte // key -> body (nil = inflight)
	prKeys   map[int][]string  // prID -> keys it owns (for eviction)
	prOrder  []int             // FIFO of PR ids
	maxPRs   int
}

func newDiffBodyCache(maxPRs int) *diffBodyCache {
	if maxPRs < 1 {
		maxPRs = 1
	}
	return &diffBodyCache{
		bodies: map[string][]byte{},
		prKeys: map[int][]string{},
		maxPRs: maxPRs,
	}
}

// Get returns (body, present). present is true even when body is nil
// (an inflight reservation), so callers can distinguish "not started"
// from "started but not done."
func (c *diffBodyCache) Get(key string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	b, ok := c.bodies[key]
	return b, ok
}

// Reserve marks key as inflight (body=nil) under prID. Subsequent
// Get(key) returns (nil, true) so the prefetcher won't re-issue.
func (c *diffBodyCache) Reserve(prID int, key string) {
	if c == nil {
		return
	}
	if _, exists := c.bodies[key]; exists {
		return
	}
	c.bodies[key] = nil
	c.trackKey(prID, key)
}

// Set stores a body under prID and evicts the oldest PR if we're over
// the limit. nil bodies are treated as failed fetches and dropped.
func (c *diffBodyCache) Set(prID int, key string, body []byte) {
	if c == nil {
		return
	}
	if body == nil {
		delete(c.bodies, key)
		return
	}
	_, was := c.bodies[key]
	c.bodies[key] = body
	if !was {
		c.trackKey(prID, key)
	}
	c.evictIfNeeded()
}

// Drop removes a key (e.g. on fetch error) without touching PR bookkeeping.
func (c *diffBodyCache) Drop(key string) {
	if c == nil {
		return
	}
	delete(c.bodies, key)
}

// ClearPR removes every body associated with prID (used by Refresh).
func (c *diffBodyCache) ClearPR(prID int) {
	if c == nil {
		return
	}
	for _, k := range c.prKeys[prID] {
		delete(c.bodies, k)
	}
	delete(c.prKeys, prID)
	for i, id := range c.prOrder {
		if id == prID {
			c.prOrder = append(c.prOrder[:i], c.prOrder[i+1:]...)
			break
		}
	}
}

func (c *diffBodyCache) trackKey(prID int, key string) {
	if _, ok := c.prKeys[prID]; !ok {
		c.prOrder = append(c.prOrder, prID)
	}
	c.prKeys[prID] = append(c.prKeys[prID], key)
}

func (c *diffBodyCache) evictIfNeeded() {
	for len(c.prOrder) > c.maxPRs {
		oldest := c.prOrder[0]
		c.prOrder = c.prOrder[1:]
		for _, k := range c.prKeys[oldest] {
			delete(c.bodies, k)
		}
		delete(c.prKeys, oldest)
	}
}
