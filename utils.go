package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// this is the message you see now and
// then. its probably a wasteful use
// of mutexes but who's going to see
// this anyway.
func printStats(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		statsMutex.Lock()
		fmt.Println(`
                 _                _     _ 
  __ _ _ __ __ _| | ___ __  _ __ (_) __| |
 / _' | '__/ _' | |/ / '_ \| '_ \| |/ _' |
| (_| | | | (_| |   <| | | | | | | | (_| |
 \__,_|_|  \__,_|_|\_\_| |_|_| |_|_|\__,_|
 https://github.com/donuts-are-good/araknnid
 araknnid ` + semverInfo)
		fmt.Printf(brightcyan+"\nProcessed URLs: %d\n"+nc, stats.processedURLs)
		fmt.Printf(brightcyan+"Max LRU Cache Len: %d\n\n"+nc, maxCacheEntries)
		fmt.Printf(brightcyan+"Errors: %d\n\n"+nc, stats.errors)

		time.Sleep(2 * time.Second)
		statsMutex.Unlock()
	}
}

// this is a link cleaner thing that
// makes sure that the link in hand
// does't pattern match anything in
// ignore.txt
func processURL(url string, ignoreList []string, cache *LRUCache, db *sqlx.DB, depth int) {

	// check if the url matches a pattern
	// on the ignore list
	if isIgnored(url, ignoreList) {
		return
	}

	// this is part of the cache system
	// that lets us know if the link in
	// hand matches any of the ones we've
	// already seen.
	content, ok := cache.Get(url)

	// this is for the stats counter
	statsMutex.Lock()
	stats.processedURLs++
	statsMutex.Unlock()
	if !ok {
		var err error
		content, err = crawl(url)
		if err != nil {

			// this is for the stats counter also
			stats.errors++
			log.Printf(brightred+"Error crawling URL %s: %v"+nc, url, err)
			return
		}
		cache.Set(url, content)
	}

	// we also do processing on the content
	data := processData(content)

	// assemble a link object
	link := Link{URL: url, Data: data}

	// announce it
	println(brightcyan + "ok: " + nc + link.URL)

	// filter out the small entries
	if len(link.Data) > 100 {
		_, err := db.NamedExec("INSERT OR IGNORE INTO links (url, data) VALUES (:url, :data)", &link)
		if err != nil {
			log.Printf("Error inserting link into database: %v", err)
		}
	}

	// get the links from the link
	newLinks := extractLinks(content, url, depth)
	for _, newLink := range newLinks {
		processURL(newLink, ignoreList, cache, db, depth-1)
	}
}

// remove duplicates from the db
func deduplicateLinks(db *sqlx.DB) error {
	rowsBefore, err := db.Query("SELECT COUNT(*) FROM links")
	if err != nil {
		return fmt.Errorf(brightred+"failed to count rows before deduplication: %v"+nc, err)
	}
	defer rowsBefore.Close()

	var countBefore int
	if rowsBefore.Next() {
		if err := rowsBefore.Scan(&countBefore); err != nil {
			return fmt.Errorf(brightred+"failed to scan count before deduplication: %v"+nc, err)
		}
	}

	sql := `DELETE FROM links WHERE id NOT IN (
		SELECT MIN(id) FROM links GROUP BY url
		);`
	result, err := db.Exec(sql)
	if err != nil {
		return fmt.Errorf(brightred+"failed to deduplicate links: %v"+nc, err)
	}

	_, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf(brightred+"failed to get rows affected by deduplication: %v"+nc, err)
	}

	rowsAfter, err := db.Query("SELECT COUNT(*) FROM links")
	if err != nil {
		return fmt.Errorf(brightred+"failed to count rows after deduplication: %v"+nc, err)
	}
	defer rowsAfter.Close()

	var countAfter int
	if rowsAfter.Next() {
		if err := rowsAfter.Scan(&countAfter); err != nil {
			return fmt.Errorf(brightred+"failed to scan count after deduplication: %v"+nc, err)
		}
	}

	fmt.Printf(brightcyan+"%sRows before deduplication: %d\n"+nc, brightcyan, countBefore)
	fmt.Printf(brightcyan+"Rows removed: %d\n"+nc, countBefore-countAfter)
	fmt.Printf(brightcyan+"Rows after deduplication: %d%s\n"+nc, countAfter, nc)

	return nil
}

// determine if the item matches a pattern on the ignore
// list
func isIgnored(url string, ignoreList []string) bool {
	for _, ignoredURL := range ignoreList {
		if strings.Contains(url, ignoredURL) {
			return true
		}
	}
	return false
}

// read the ignore list
func readIgnoreList() ([]string, error) {
	file, err := os.Open("ignore.txt")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}
