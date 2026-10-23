package server

import (
	"container/list"
	"sync"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/font"
)

// compiled is a compiled template with its own font registry. The
// registry is per template, so that fonts of one package do not leak into
// other templates, and it is shared across all requests.
type compiled struct {
	tpl   *pdx.Template
	fonts *font.Registry
}

// lru holds the most recently used compiled templates. The key is
// template, version and language, versions are immutable.
type lru struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List
	items map[string]*list.Element
}

type lruEntry struct {
	key string
	val *compiled
}

func newLRU(n int) *lru {
	return &lru{cap: n, ll: list.New(), items: map[string]*list.Element{}}
}

func (c *lru) get(key string) (*compiled, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.ll.MoveToFront(e)
		return e.Value.(*lruEntry).val, true
	}
	return nil, false
}

func (c *lru) put(key string, v *compiled) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.ll.MoveToFront(e)
		e.Value.(*lruEntry).val = v
		return
	}
	c.items[key] = c.ll.PushFront(&lruEntry{key, v})
	for c.ll.Len() > c.cap {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.items, last.Value.(*lruEntry).key)
	}
}

func (c *lru) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// flight executes concurrent calls with the same key only once.
type flight struct {
	mu    sync.Mutex
	calls map[string]*call
}

type call struct {
	wg  sync.WaitGroup
	val any
	err error
}

func (g *flight) Do(key string, fn func() (any, error)) (any, error, bool) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = map[string]*call{}
	}
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err, true
	}
	c := &call{}
	c.wg.Add(1)
	g.calls[key] = c
	g.mu.Unlock()
	c.val, c.err = fn()
	c.wg.Done()
	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	return c.val, c.err, false
}
