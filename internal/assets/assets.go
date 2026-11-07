// Package assets embeds the seed JSON and the images the app ships with.
package assets

import (
	"embed"
	"encoding/json"
	"io/fs"
	"sync"
)

//go:embed data images
var FS embed.FS

// Data reads a file under data/.
func Data(name string) ([]byte, error) { return FS.ReadFile("data/" + name) }

// Image reads a file under images/.
func Image(name string) ([]byte, error) { return FS.ReadFile("images/" + name) }

// DataFS is the data directory as a filesystem.
func DataFS() fs.FS {
	sub, err := fs.Sub(FS, "data")
	if err != nil {
		panic(err)
	}
	return sub
}

// Manifest names each catalog item's image and each category's image.
type Manifest struct {
	Version     int                     `json:"version"`
	GeneratedAt string                  `json:"generatedAt"`
	Items       map[string]ManifestItem `json:"items"`
	Categories  map[string]ManifestItem `json:"categories"`
}

// ManifestItem is one image entry.
type ManifestItem struct {
	Slug   string `json:"slug"`
	Image  string `json:"image"`
	Status string `json:"status"`
}

var (
	manifestOnce sync.Once
	manifest     Manifest
	manifestErr  error
)

// LoadManifest parses images/manifest.json once.
func LoadManifest() (Manifest, error) {
	manifestOnce.Do(func() {
		b, err := Image("manifest.json")
		if err != nil {
			manifestErr = err
			return
		}
		manifestErr = json.Unmarshal(b, &manifest)
	})
	return manifest, manifestErr
}

// CategoryImage returns the embedded image path for a category name.
func CategoryImage(name string) (string, bool) {
	m, err := LoadManifest()
	if err != nil {
		return "", false
	}
	entry, ok := m.Categories[name]
	if !ok || entry.Image == "" {
		return "", false
	}
	return entry.Image, true
}

// ItemImage returns the embedded image path for a catalog item name.
func ItemImage(name string) (string, bool) {
	m, err := LoadManifest()
	if err != nil {
		return "", false
	}
	entry, ok := m.Items[name]
	if !ok || entry.Image == "" {
		return "", false
	}
	return entry.Image, true
}
