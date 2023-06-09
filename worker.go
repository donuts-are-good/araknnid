package main

import (
	"sync"

	"github.com/jmoiron/sqlx"
)

func worker(id int, jobs <-chan string, wg *sync.WaitGroup, ignoreList []string, cache *LRUCache, db *sqlx.DB, depthFlag int) {
	defer wg.Done()
	for url := range jobs {
		processURL(id, url, ignoreList, cache, db, depthFlag)
	}
}
