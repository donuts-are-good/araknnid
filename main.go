package main

import (
	"bytes"
	"flag"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/html"
)

const userAgent = "Araknnid/0.1"

type Link struct {
	ID   int    `db:"id"`
	URL  string `db:"url"`
	Data string `db:"data"`
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

	queue := []string{}

	if *urlFlag == "" {
		rows, err := db.Query("SELECT url FROM links")
		if err != nil {
			log.Fatal("Failed to query links from database")
		}

		for rows.Next() {
			var url string
			rows.Scan(&url)
			queue = append(queue, url)
		}
	} else {
		queue = append(queue, *urlFlag)
	}

	for len(queue) > 0 {
		url := queue[0]
		queue = queue[1:]

		content, err := crawl(url)
		if err != nil {
			log.Printf("Error crawling URL %s: %v", url, err)
			continue
		}

		data := processData(content)

		link := Link{URL: url, Data: data}

		println("ok: " + link.URL)
		if len(link.Data) > 100 {
			// println("db: " + link.URL)
			_, err = db.NamedExec("INSERT OR IGNORE INTO links (url, data) VALUES (:url, :data)", &link)
			if err != nil {
				log.Printf("Error inserting link into database: %v", err)
			}
		}

		newLinks := extractLinks(content, url, *depthFlag)
		queue = append(queue, newLinks...)
	}
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
