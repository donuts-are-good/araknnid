package main

import (
	"log"
	"sync"

	"github.com/jmoiron/sqlx"
)

func worker(id int, jobs <-chan string, wg *sync.WaitGroup, ignoreList []string, cache *LRUCache, db *sqlx.DB, depthFlag int) {
	defer wg.Done()
	for url := range jobs {
		log.Printf("Worker %d processing URL: %s\n", id, url)
		processURL(url, ignoreList, cache, db, depthFlag)
	}
}
