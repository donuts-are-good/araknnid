package main

import "container/list"

func (c *LRUCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evictList.MoveToFront(elem)
		return elem.Value.(*cacheItem).value, true
	}

	return "", false
}

func (c *LRUCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evictList.MoveToFront(elem)
		elem.Value.(*cacheItem).value = value
		return
	}

	elem := c.evictList.PushFront(&cacheItem{key: key, value: value})
	c.items[key] = elem

	if c.evictList.Len() > c.size {
		c.removeOldest()
	}
}

func (c *LRUCache) removeOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

func (c *LRUCache) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	kv := e.Value.(*cacheItem)
	delete(c.items, kv.key)
}

func NewLRUCache(size int) *LRUCache {
	return &LRUCache{
		size:      size,
		evictList: list.New(),
		items:     make(map[string]*list.Element, size),
	}
}
