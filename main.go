package main

import (
	"log"

	// Make Windows choose Simplified Chinese fonts for Han characters.
	_ "github.com/egoist/mygo/locales/zh-Hans"

	"github.com/gij072858-cmd/foodbox-go/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
