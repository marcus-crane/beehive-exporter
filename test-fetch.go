package main

import (
	"fmt"
	"log"
	"os"

	"github.com/marcuswhybrow/beehive-exports/internal/fetcher"
)

func main() {
	f := fetcher.New()
	html, err := f.FetchURL("https://www.beehive.govt.nz/releases?page=0")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Fetched %d bytes\n", len(html))
	fmt.Println("First 500 chars:")
	if len(html) > 500 {
		fmt.Println(html[:500])
	} else {
		fmt.Println(html)
	}

	// Save to file
	os.WriteFile("debug-page.html", []byte(html), 0644)
	fmt.Println("\nSaved full HTML to debug-page.html")
}
