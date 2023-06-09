package main

import (
	"flag"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

var (

	// v. major. minor. patch
	semverInfo = "v0.1.0"

	// the cache tracks which links you've seen
	maxCacheEntries = 10000

	// this is the random slice of db links you
	// pull when you don't specify a --url on launch
	maxRandomLinks = 100
	fetchFrequency = 10 * maxRandomLinks

	// sensible user agent is sensible
	// give people an avenue to contact in case they want their
	// domains added to the ignore list.
	userAgent = "Mozilla/5.0 (compatible; Araknnid/0.1.0; +github.com/donuts-are-good/araknnid)"
)

// lets get it started
func main() {

	// check for flags
	// urlFlag is a starting off point
	// that you can define
	urlFlag := flag.String("url", "", "URL to start crawling from")

	// link depth
	depthFlag := flag.Int("depth", 1, "Maximum depth for the crawler")

	// number of workers
	workersFlag := flag.Int("workers", 1, "Number of concurrent workers")

	flag.Parse()

	// make a connection to the db
	db, err := sqlx.Connect("sqlite3", "spider.db")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// if our db wasn't setup, set it up
	schema := `CREATE TABLE IF NOT EXISTS links (id INTEGER PRIMARY KEY, url TEXT UNIQUE, data TEXT);`
	db.MustExec(schema)

	// check the ignorelist
	ignoreList, err := readIgnoreList()
	if err != nil {
		log.Fatalf(brightred+"Failed to read ignore list: %v"+nc, err)
	}

	// prepare the cache for the scanned links
	cache := NewLRUCache(maxCacheEntries)

	// print a status message on an interval
	go printStats(10*time.Second, *workersFlag, cache)

	// check the db for duplicate links
	deduplicateLinks(db)

	// if there was no url specified, grab some
	// random urls from the db and start there.

	if *urlFlag == "" {
		for {
			// Fetch random URLs
			rows, err := db.Query("SELECT url FROM links ORDER BY RANDOM() LIMIT " + strconv.Itoa(maxRandomLinks))
			if err != nil {
				log.Fatal(brightred + "Failed to query links from database" + nc)
			}

			// Clear previous URLs and add new random URLs
			urls := make(chan string, maxRandomLinks)
			counter := 0
			for rows.Next() {
				var url string
				rows.Scan(&url)
				urls <- url
				counter++
				if counter%fetchFrequency == 0 {
					break
				}
			}
			close(urls)

			// Create workers
			var wg sync.WaitGroup
			wg.Add(*workersFlag)
			for i := 0; i < *workersFlag; i++ {
				go worker(i, urls, &wg, ignoreList, cache, db, *depthFlag)
			}
			wg.Wait()
		}
	} else {
		processURL(0, *urlFlag, ignoreList, cache, db, *depthFlag)
	}
}
