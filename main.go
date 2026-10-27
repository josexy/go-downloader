package main

import (
	"log"

	"go-downloader/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
