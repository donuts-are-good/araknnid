package main

import (
	"container/list"
	"net/http"
	"sync"
)

type LRUCache struct {
	mu sync.Mutex

	size      int
	evictList *list.List
	items     map[string]*list.Element
}

type cacheItem struct {
	key   string
	value string
}

type userAgentTransport struct {
	transport http.RoundTripper
	userAgent string
}


// this is a row entry in the db
// it made sense at the time to
// call it link but in hindsight
// it could have a better name
type Link struct {
	ID   int    `db:"id"`
	URL  string `db:"url"`
	Data string `db:"data"`
}

var (
	brightred  = "\033[1;31m"
	brightcyan = "\033[1;36m"
	nc         = "\033[0m"

	// this mutex stuff is for the stats
	// if you notice sometimes the terminal
	// prints how many errors and links it
	// has seen since you started the app
	// and produces version info. that's this
	statsMutex sync.Mutex
	stats      = struct {
		processedURLs int
		errors        int
		cacheItems    int
	}{
		processedURLs: 0,
		errors:        0,
		cacheItems:    0,
	}
)
