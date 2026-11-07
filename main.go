package main

import (
	"log"

	"github.com/egoist/mygo"
	// Make Windows choose Simplified Chinese fonts for Han characters.
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/plugins/updater/native"

	"github.com/gij072858-cmd/foodbox-go/internal/app"
)

func main() {
	// The app is native UI, so the updater's window is native too: no
	// WebView2 is needed for it. Releases are signed with the key in CI.
	mygo.Use(native.Plugin)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
