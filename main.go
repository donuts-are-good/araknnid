package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/html"
)

const (
	userAgent     = "Mozilla/5.0 (compatible; Araknnid/0.1; +github.com/donuts-are-good/araknnid)"
	workerCount   = 12
	queueCapacity = 100000000
)

var (
	brightred   = "\033[1;31m"
	brightgreen = "\033[1;32m"
	brightcyan  = "\033[1;36m"
	nc          = "\033[0m"
	urlQueue    = []string{}
)

type Link struct {
	ID   int    `db:"id"`
	URL  string `db:"url"`
	Data string `db:"data"`
}

type Cache struct {
	sync.Mutex
	m map[string]string
}

func (c *Cache) Get(url string) (string, bool) {
	c.Lock()
	defer c.Unlock()
	val, ok := c.m[url]
	return val, ok
}

func (c *Cache) Set(url, content string) {
	c.Lock()
	defer c.Unlock()
	c.m[url] = content
}

func printQueueStatus(urlCh chan string) {
	queueSize := len(urlCh)
	fmt.Printf(brightcyan+"Queue size: %d\nItems left: %d\n"+nc, queueCapacity, queueSize)
}
func main() {
	urlFlag := flag.String("url", "", "URL to start crawling from")
	depthFlag := flag.Int("depth", 1, "Maximum depth for the crawler")
	flag.Parse()

	db, err := sqlx.Connect("sqlite3", "spider.db")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	schema := `CREATE TABLE IF NOT EXISTS links (id INTEGER PRIMARY KEY, url TEXT UNIQUE, data TEXT);`
	db.MustExec(schema)

	ignoreList, err := readIgnoreList()
	if err != nil {
		log.Fatalf(brightred+"Failed to read ignore list: %v"+nc, err)
	}

	urlCh := make(chan string, queueCapacity)
	doneCh := make(chan struct{})
	wg := &sync.WaitGroup{}
	urlsWg := &sync.WaitGroup{}

	cache := Cache{m: make(map[string]string)}
	deduplicateLinks(db)

	queueSizeTicker := time.NewTicker(time.Minute)
	defer queueSizeTicker.Stop()
	go func() {
		for {
			printQueueStatus(urlCh)
			time.Sleep(10 * time.Second)
		}
	}()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			urlProcessed := 0
			for {

				select {
				case url := <-urlCh:
					queueSize := len(urlCh)
					fmt.Printf(brightcyan+"Queue size: %d\nItems left: %d\n"+nc, queueCapacity, queueSize)
					if queueCapacity <= queueSize {
						log.Println("Resetting queue...")
						urlQueue = []string{}
					}
					urlQueue = append(urlQueue, url)
					if urlProcessed%10 == 0 {
						shuffle(urlQueue)
					}
					urlProcessed++

					// Clear the queue if it's full
					if len(urlQueue) >= queueCapacity {
						urlQueue = []string{}
					}

					currentURL := urlQueue[0]
					urlQueue = urlQueue[1:]

					if isIgnored(currentURL, ignoreList) {
						urlsWg.Done()
						continue
					}

					content, ok := cache.Get(currentURL)
					if !ok {
						var err error
						content, err = crawl(currentURL)
						if err != nil {
							log.Printf(brightred+"Error crawling URL %s: %v"+nc, currentURL, err)
							urlsWg.Done()
							continue
						}
						cache.Set(currentURL, content)
					}

					data := processData(content)

					link := Link{URL: currentURL, Data: data}

					println("ok: " + link.URL)
					if len(link.Data) > 100 {
						_, err = db.NamedExec("INSERT OR IGNORE INTO links (url, data) VALUES (:url, :data)", &link)
						if err != nil {
							log.Printf("Error inserting link into database: %v", err)
						}
					}

					newLinks := extractLinks(content, currentURL, *depthFlag)
					for _, newLink := range newLinks {
						urlsWg.Add(1)
						if len(urlQueue) >= queueCapacity {
							urlQueue = []string{}
						}
						urlCh <- newLink
					}
					urlsWg.Done()
				case <-doneCh:
					return
				}
			}
		}()
	}

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

		shuffle(urls)

		for _, url := range urls {
			urlsWg.Add(1)
			urlCh <- url
		}
	} else {
		urlsWg.Add(1)
		urlCh <- *urlFlag
	}

	urlsWg.Wait()
	close(doneCh)
	wg.Wait()
}
func shuffle(urls []string) {
	for i := len(urls) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		urls[i], urls[j] = urls[j], urls[i]
	}
}
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

	fmt.Printf("%sRows before deduplication: %d\n", brightcyan, countBefore)
	fmt.Printf("Rows removed: %d\n", countBefore-countAfter)
	fmt.Printf("Rows after deduplication: %d%s\n", countAfter, nc)

	time.Sleep(5 * time.Second)
	return nil
}

func isIgnored(url string, ignoreList []string) bool {
	for _, ignoredURL := range ignoreList {
		if strings.Contains(url, ignoredURL) {
			return true
		}
	}
	return false
}

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
func crawl(url string) (string, error) {
	client := &http.Client{
		Transport: userAgentTransport{
			transport: http.DefaultTransport,
			userAgent: userAgent,
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

type userAgentTransport struct {
	transport http.RoundTripper
	userAgent string
}

func (uat userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", uat.userAgent)
	return uat.transport.RoundTrip(req)
}
func processData(content string) string {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		log.Printf("Error parsing HTML: %v", err)
		return ""
	}

	var buf bytes.Buffer
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode && isContentNode(n.Parent) {
			buf.WriteString(n.Data)
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	reg, _ := regexp.Compile("[^a-zA-Z0-9.,?!]+")
	processed := reg.ReplaceAllString(buf.String(), " ")
	processed = strings.ToLower(processed)
	return processed
}

func isContentNode(n *html.Node) bool {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "p", "h1", "h2", "h3", "h4", "h5", "h6", "code", "pre":
			return true
		}
	}
	return false
}

func extractLinks(content, base string, depth int) []string {
	if depth <= 0 {
		return nil
	}

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		log.Printf("Error parsing HTML: %v", err)
		return nil
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		log.Printf("Error parsing base URL: %v", err)
		return nil
	}

	var links []string
	var f func(*html.Node, int)
	f = func(n *html.Node, depth int) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					linkURL, err := baseURL.Parse(a.Val)
					if err == nil {
						links = append(links, linkURL.String())
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c, depth-1)
		}
	}
	f(doc, depth)
	return links
}
