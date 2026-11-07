package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// TestM6Screenshots writes the M6 evidence: 食历、年度回顾、我的、设置、引导与帮助.
// Like the other milestone screenshots it is a test, so the images are
// regenerated with the code.
func TestM6Screenshots(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	tt.SetLocale("zh-Hans")
	tt.Frame()

	clickText(t, tt, "食历")
	capture(t, tt, filepath.Join(dir, "m6-feed.png"))

	clickText(t, tt, "年度回顾")
	capture(t, tt, filepath.Join(dir, "m6-review.png"))

	clickText(t, tt, "我的")
	capture(t, tt, filepath.Join(dir, "m6-profile.png"))

	clickText(t, tt, "设置")
	capture(t, tt, filepath.Join(dir, "m6-settings.png"))

	clickText(t, tt, "帮助")
	capture(t, tt, filepath.Join(dir, "m6-help.png"))

	a.prefs.onboarding = onboardingFrom(domain.UserProfile{})
	tt.Frame()
	capture(t, tt, filepath.Join(dir, "m6-onboarding.png"))
}
