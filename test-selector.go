package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/marcuswhybrow/beehive-exports/internal/fetcher"
)

func main() {
	f := fetcher.New()
	html, err := f.FetchURL("https://www.beehive.govt.nz/release/making-it-easier-find-cheap-power-plans")
	if err != nil {
		log.Fatal(err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Fatal(err)
	}

	// Test different selectors
	fmt.Println("Testing selectors:")

	sel1 := doc.Find("div.field.field--name-body")
	fmt.Printf("div.field.field--name-body: %d matches\n", sel1.Length())
	sel1.Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		fmt.Printf("  Match %d: %d chars\n", i, len(text))
		if len(text) > 100 {
			fmt.Printf("    Preview: %s...\n", text[:100])
		} else if len(text) > 0 {
			fmt.Printf("    Text: %s\n", text)
		}
	})

	sel2 := doc.Find(".field--name-body")
	fmt.Printf(".field--name-body: %d matches\n", sel2.Length())
	if sel2.Length() > 0 {
		fmt.Printf("  First match class: %s\n", sel2.First().AttrOr("class", ""))
		fmt.Printf("  First 100 chars of text: %s\n", sel2.First().Text()[:100])
	}

	sel3 := doc.Find("[class*='field--name-body']")
	fmt.Printf("[class*='field--name-body']: %d matches\n", sel3.Length())
	if sel3.Length() > 0 {
		fmt.Printf("  First match class: %s\n", sel3.First().AttrOr("class", ""))
		text := sel3.First().Text()
		if len(text) > 100 {
			fmt.Printf("  First 100 chars of text: %s\n", text[:100])
		} else {
			fmt.Printf("  Text: %s\n", text)
		}
	}
}
