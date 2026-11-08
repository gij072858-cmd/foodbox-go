package app

// The first-run questionnaire keeps its actions in the fixed header: at the
// smallest supported window (1024x700) the buttons must be clickable without
// scrolling, because the questions themselves are taller than the window.

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

func onboardingTestApp() *App {
	a := newTestApp()
	a.prefs.onboarding = onboardingFrom(domain.UserProfile{})
	a.prefs.onboarding.required = true
	return a
}

func TestOnboardingActionsReachableAtMinSize(t *testing.T) {
	a := onboardingTestApp()
	tt := ui.NewTester(a.View, 1024, 700)
	if !tt.HasText("开始使用食匣") {
		t.Fatalf("questionnaire not shown: %q", tt.Texts())
	}
	if err := tt.Click("跳过"); err != nil {
		t.Fatalf("skip button is unreachable at 1024x700: %v", err)
	}
	settle(tt)
	if a.prefs.onboarding.required {
		t.Error("skip did not finish the questionnaire")
	}
	if !tt.HasText("仓库") {
		t.Errorf("shell not shown after skip: %q", tt.Texts())
	}
}

// The household steppers are the app's click-driven design.Stepper, not the
// framework's press-only arrows: each click moves the number and the total.
func TestOnboardingHouseholdSteppers(t *testing.T) {
	a := onboardingTestApp()
	tt := ui.NewTester(a.View, 1024, 700)
	if err := tt.Click("下一步"); err != nil {
		t.Fatalf("household stop unreachable: %v", err)
	}
	settle(tt)
	if !tt.HasText("家里几口人") {
		t.Fatalf("household stop not shown: %q", tt.Texts())
	}
	for _, label := range []string{"增加成人人数", "增加小孩人数", "增加老人人数"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("stepper button %q is missing", label)
		}
	}
	if err := tt.Click("增加成人人数"); err != nil {
		t.Fatalf("adult plus unreachable: %v", err)
	}
	settle(tt)
	if a.prefs.onboarding.adults != 3 {
		t.Errorf("adults after one plus = %d, want 3", a.prefs.onboarding.adults)
	}
	if err := tt.Click("增加成人人数"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.prefs.onboarding.adults != 4 || !tt.HasText("一共 4 位") {
		t.Errorf("adults after two plus = %d, total shown = %v",
			a.prefs.onboarding.adults, tt.HasText("一共 4 位"))
	}
	if err := tt.Click("减少成人人数"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.prefs.onboarding.adults != 3 {
		t.Errorf("adults after minus = %d, want 3", a.prefs.onboarding.adults)
	}
}

func TestOnboardingFinishSavesAnswers(t *testing.T) {
	a := onboardingTestApp()
	tt := ui.NewTester(a.View, 1024, 700)
	// Pick 3 stoves on the first stop, the halal restriction on the third,
	// then finish on the last.
	if err := tt.Click("3 个"); err != nil {
		t.Fatalf("stove control unreachable: %v", err)
	}
	tt.Advance(100 * time.Millisecond)
	if err := tt.Click("下一步"); err != nil {
		t.Fatalf("next step unreachable: %v", err)
	}
	settle(tt)
	if err := tt.Click("下一步"); err != nil {
		t.Fatalf("next step unreachable: %v", err)
	}
	settle(tt)
	if err := tt.Click("清真"); err != nil {
		t.Fatalf("restriction chip unreachable: %v", err)
	}
	tt.Advance(100 * time.Millisecond)
	if err := tt.Click("下一步"); err != nil {
		t.Fatalf("next step unreachable: %v", err)
	}
	settle(tt)
	if err := tt.Click("完成并进入"); err != nil {
		t.Fatalf("finish button is unreachable at 1024x700: %v", err)
	}
	settle(tt)
	if a.prefs.onboarding.required {
		t.Error("finish did not close the questionnaire")
	}
	p := a.prefs.onboarding.toProfile()
	if p.StoveCount != 3 {
		t.Errorf("stove count = %d, want 3", p.StoveCount)
	}
	if p.Restrictions != "halal" {
		t.Errorf("restrictions = %q, want halal", p.Restrictions)
	}
	if !p.Onboarded {
		t.Error("profile not marked onboarded")
	}
}
