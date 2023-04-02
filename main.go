package main

import (
	"flag"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

var (

	// v. major. minor. patch
	semverInfo = "v0.1.0"

	// the cache tracks which links you've seen
	maxCacheEntries = 100000

	// this is the random slice of db links you
	// pull when you don't specify a --url on launch
	maxRandomLinks = 1000

	// sensible user agent is sensible
	// give people an avenue to contact in case they want their
	// domains added to the ignore list.
	userAgent = "Mozilla/5.0 (compatible; Araknnid/0.1.0; +github.com/donuts-are-good/araknnid)"
)

// lets get it started
func main() {

	// print a status message on an interval
	go printStats(30 * time.Second)

	// check for flags
	// urlFlag is a starting off point
	// that you can define
	urlFlag := flag.String("url", "", "URL to start crawling from")

	// link depth
	depthFlag := flag.Int("depth", 1, "Maximum depth for the crawler")
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

	// check the db for duplicate links
	deduplicateLinks(db)

	// if there was no url specified, grab some
	// random urls from the db and start there.
	if *urlFlag == "" {
		rows, err := db.Query("SELECT url FROM links")
		if err != nil {
			log.Fatal(brightred + "Failed to query links from database" + nc)
		}

		urls := []string{}
		for rows.Next() {
			var url string
			rows.Scan(&url)
			urls = append(urls, url)
		}

		// shuffle them sometimes so we're not
		// stuck on the same huge site forever
		shuffle(urls)

		// for each url we scoop up, scrub it
		for i, url := range urls {
			if i >= maxRandomLinks {
				break
			}
			processURL(url, ignoreList, cache, db, *depthFlag)
		}
	} else {
		processURL(*urlFlag, ignoreList, cache, db, *depthFlag)
	}
}
