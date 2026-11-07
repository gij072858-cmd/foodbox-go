package seed_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/data/seed"
	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
)

// TestImportPerformance is the plan's first-launch gate: the seed import
// (944 recipes, 6,930 steps) must finish in under 10 seconds.
func TestImportPerformance(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "perf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	summary, err := seed.Import(ctx, store, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Recipes != 944 || summary.Steps != 6930 {
		t.Fatalf("imported %d recipes / %d steps", summary.Recipes, summary.Steps)
	}
	if elapsed > 10*time.Second {
		t.Errorf("seed import took %v, budget 10s", elapsed)
	}
	t.Logf("seed import: %v", elapsed)

	// The second launch skips the import and stays fast.
	start = time.Now()
	again, err := seed.Import(ctx, store, nil)
	skipped := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Skipped {
		t.Error("second import did not skip")
	}
	if skipped > time.Second {
		t.Errorf("skipped import took %v", skipped)
	}
}
