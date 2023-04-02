package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// this is the heart of the spider
func crawl(url string) (string, error) {
	client := &http.Client{
		Transport: userAgentTransport{
			transport: http.DefaultTransport,
			userAgent: userAgent,
		},
		Timeout: 4 * time.Second,
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

func (uat userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", uat.userAgent)
	return uat.transport.RoundTrip(req)
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
