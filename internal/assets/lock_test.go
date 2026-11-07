package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"testing"
	"time"
)

// updateLock rewrites assets.lock.json from the embedded files. Run it after
// tools/sync-assets.ps1 copies new data in:
//
//	go test ./internal/assets -run TestAssetsLock -args -update-assets-lock
var updateLock = flag.Bool("update-assets-lock", false, "rewrite assets.lock.json")

// lockFile pins the shipped asset bytes so a data change cannot slip in
// unnoticed: the test recomputes every hash and compares.
type lockFile struct {
	Version     int               `json:"version"`
	GeneratedAt string            `json:"generatedAt"`
	Data        map[string]string `json:"data"`
	Images      struct {
		Count  int    `json:"count"`
		SHA256 string `json:"sha256"`
	} `json:"images"`
}

const lockPath = "assets.lock.json"

func computeLock(tb testing.TB) lockFile {
	tb.Helper()
	var lock lockFile
	lock.Version = 1
	lock.Data = make(map[string]string)

	entries, err := fs.ReadDir(FS, "data")
	if err != nil {
		tb.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := fs.ReadFile(FS, "data/"+e.Name())
		if err != nil {
			tb.Fatal(err)
		}
		sum := sha256.Sum256(b)
		lock.Data[e.Name()] = hex.EncodeToString(sum[:])
	}

	var paths []string
	if err := fs.WalkDir(FS, "images", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		tb.Fatal(err)
	}
	sort.Strings(paths)
	agg := sha256.New()
	for _, path := range paths {
		b, err := fs.ReadFile(FS, path)
		if err != nil {
			tb.Fatal(err)
		}
		sum := sha256.Sum256(b)
		fmt.Fprintf(agg, "%s\x00%s\n", path, hex.EncodeToString(sum[:]))
	}
	lock.Images.Count = len(paths)
	lock.Images.SHA256 = hex.EncodeToString(agg.Sum(nil))
	return lock
}

// TestAssetsLock fails when the shipped data or images no longer match the
// pinned hashes. Pass -update-assets-lock to accept a deliberate change.
func TestAssetsLock(t *testing.T) {
	current := computeLock(t)
	if *updateLock {
		current.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
		b, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lockPath, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s: %d data files, %d images", lockPath, len(current.Data), current.Images.Count)
		return
	}

	b, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update-assets-lock to create it)", lockPath, err)
	}
	var pinned lockFile
	if err := json.Unmarshal(b, &pinned); err != nil {
		t.Fatalf("parse %s: %v", lockPath, err)
	}
	if pinned.Version != current.Version {
		t.Fatalf("lock version = %d, want %d", pinned.Version, current.Version)
	}
	if len(pinned.Data) != len(current.Data) {
		t.Fatalf("lock lists %d data files, embedded %d", len(pinned.Data), len(current.Data))
	}
	for name, sum := range current.Data {
		if pinned.Data[name] != sum {
			t.Errorf("data/%s hash changed (run with -update-assets-lock after a deliberate sync)", name)
		}
	}
	if pinned.Images.Count != current.Images.Count || pinned.Images.SHA256 != current.Images.SHA256 {
		t.Errorf("images changed: lock %d/%s, embedded %d/%s",
			pinned.Images.Count, pinned.Images.SHA256,
			current.Images.Count, current.Images.SHA256)
	}
}
