package nutrition

import (
	"strings"
	"testing"
)

// The Mifflin-St Jeor fixtures below are hand computed:
//
//	male   10×70 + 6.25×175 − 5×30 + 5   = 1648.75 → BMR 1649
//	TDEE   1648.75 × 1.55                = 2555.56 → 2556
//	均衡   target = TDEE, protein 70×1.2 = 84, fat 27.5% → 78, carb fills
func TestEnergyForMaleBalanced(t *testing.T) {
	e := EnergyFor(EnergyInput{
		HeightCm: 175, WeightKg: 70, Age: 30, Sex: "male",
		ActivityLevel: "moderate", Goals: []string{"均衡"},
	})
	if e.BMR != 1649 {
		t.Errorf("BMR = %d, want 1649", e.BMR)
	}
	if e.TDEE != 2556 {
		t.Errorf("TDEE = %d, want 2556", e.TDEE)
	}
	if e.TargetKcal != 2556 {
		t.Errorf("TargetKcal = %d, want 2556", e.TargetKcal)
	}
	if e.ProteinG != 84 {
		t.Errorf("ProteinG = %d, want 84", e.ProteinG)
	}
	if e.FatG != 78 {
		t.Errorf("FatG = %d, want 78", e.FatG)
	}
	if e.CarbG != 380 {
		t.Errorf("CarbG = %d, want 380", e.CarbG)
	}
	if e.Note != "" {
		t.Errorf("Note = %q, want none", e.Note)
	}
}

// female 10×55 + 6.25×160 − 5×28 − 161 = 1249
// TDEE   1249 × 1.2                    = 1498.8 → 1499
// 减脂   target = 1498.8 × 0.85        = 1274
// protein 55×1.6 = 88, fat 25% → 35, carb fills → 152
func TestEnergyForFemaleCut(t *testing.T) {
	e := EnergyFor(EnergyInput{
		HeightCm: 160, WeightKg: 55, Age: 28, Sex: "female",
		ActivityLevel: "sedentary", Goals: []string{"减脂"},
	})
	if e.BMR != 1249 || e.TDEE != 1499 || e.TargetKcal != 1274 {
		t.Errorf("BMR/TDEE/target = %d/%d/%d, want 1249/1499/1274", e.BMR, e.TDEE, e.TargetKcal)
	}
	if e.ProteinG != 88 || e.FatG != 35 || e.CarbG != 152 {
		t.Errorf("macros = %d/%d/%d, want 88/35/152", e.ProteinG, e.FatG, e.CarbG)
	}
}

// Unknown sex takes the midpoint of +5 and −161:
//
//	10×60 + 6.25×170 − 5×40 − 78 = 1384.5 → BMR 1385
//	TDEE 1384.5 × 1.725 = 2388.26        → 2388
//	增肌 target = 2388.26 × 1.1 = 2627
func TestEnergyForUnknownSexBulk(t *testing.T) {
	e := EnergyFor(EnergyInput{
		HeightCm: 170, WeightKg: 60, Age: 40, Sex: "",
		ActivityLevel: "active", Goals: []string{"增肌"},
	})
	if e.BMR != 1385 || e.TDEE != 2388 || e.TargetKcal != 2627 {
		t.Errorf("BMR/TDEE/target = %d/%d/%d, want 1385/2388/2627", e.BMR, e.TDEE, e.TargetKcal)
	}
	if e.ProteinG != 108 || e.FatG != 88 || e.CarbG != 351 {
		t.Errorf("macros = %d/%d/%d, want 108/88/351", e.ProteinG, e.FatG, e.CarbG)
	}
	if !strings.Contains(e.Note, "性别未填") {
		t.Errorf("Note = %q, want the unknown-sex caveat", e.Note)
	}
}

// Missing body data returns no numbers, only a note naming what is missing.
func TestEnergyForMissingInputs(t *testing.T) {
	e := EnergyFor(EnergyInput{Age: 30, Sex: "male", ActivityLevel: "light"})
	if e.BMR != 0 || e.TDEE != 0 || e.TargetKcal != 0 || e.ProteinG != 0 {
		t.Errorf("energy = %+v, want zeroes", e)
	}
	for _, want := range []string{"身高", "体重"} {
		if !strings.Contains(e.Note, want) {
			t.Errorf("Note = %q, want it to name %s", e.Note, want)
		}
	}
	if strings.Contains(e.Note, "年龄") {
		t.Errorf("Note = %q, should not name 年龄 when it is present", e.Note)
	}
}

// A missing activity level falls back to sedentary and says so.
func TestEnergyForUnknownActivity(t *testing.T) {
	e := EnergyFor(EnergyInput{HeightCm: 170, WeightKg: 60, Age: 30, Sex: "male"})
	if e.TDEE == 0 {
		t.Fatal("TDEE should still be computed")
	}
	if !strings.Contains(e.Note, "活动量未填") {
		t.Errorf("Note = %q, want the sedentary caveat", e.Note)
	}
	// 1.2 multiplier: 600 + 1062.5 − 150 + 5 = 1517.5 → 1518.
	if e.BMR != 1518 || e.TDEE != 1821 {
		t.Errorf("BMR/TDEE = %d/%d, want 1518/1821", e.BMR, e.TDEE)
	}
}

// Life stages add their extra energy on top of the activity-adjusted burn.
func TestEnergyForLifeStage(t *testing.T) {
	base := EnergyInput{HeightCm: 165, WeightKg: 60, Age: 30, Sex: "female", ActivityLevel: "light"}
	none := EnergyFor(base)
	for _, tc := range []struct {
		stage string
		extra int
		label string
	}{
		{"pregnancyMid", 300, "孕期"},
		{"lactation", 500, "哺乳期"},
		{"postSurgery", 200, "恢复期"},
	} {
		in := base
		in.LifeStage = tc.stage
		got := EnergyFor(in)
		if got.TDEE != none.TDEE+tc.extra {
			t.Errorf("%s TDEE = %d, want %d", tc.stage, got.TDEE, none.TDEE+tc.extra)
		}
		if !strings.Contains(got.Note, tc.label) {
			t.Errorf("%s Note = %q, want %s", tc.stage, got.Note, tc.label)
		}
	}
	// 备孕 carries no energy adjustment.
	in := base
	in.LifeStage = "preparingPregnancy"
	if got := EnergyFor(in); got.TDEE != none.TDEE {
		t.Errorf("备孕 TDEE = %d, want unchanged %d", got.TDEE, none.TDEE)
	}
}

// Fat sits inside 25–30% of the target and the macros add up to it.
func TestEnergyMacrosBalance(t *testing.T) {
	for _, goal := range []string{"减脂", "增肌", "均衡", "控糖", "低嘌呤"} {
		e := EnergyFor(EnergyInput{
			HeightCm: 172, WeightKg: 68, Age: 35, Sex: "male",
			ActivityLevel: "active", Goals: []string{goal},
		})
		fatShare := float64(e.FatG) * 9 / float64(e.TargetKcal)
		if fatShare < 0.24 || fatShare > 0.31 {
			t.Errorf("%s fat share = %.2f, want 0.25–0.30", goal, fatShare)
		}
		kcal := e.ProteinG*4 + e.FatG*9 + e.CarbG*4
		if diff := kcal - e.TargetKcal; diff < -4 || diff > 4 {
			t.Errorf("%s macros = %d kcal, target %d", goal, kcal, e.TargetKcal)
		}
	}
}

// A hand-set target re-derives the macros but keeps the basal numbers.
func TestApplyTarget(t *testing.T) {
	in := EnergyInput{HeightCm: 175, WeightKg: 70, Age: 30, Sex: "male", ActivityLevel: "moderate"}
	e := EnergyFor(in)
	got := ApplyTarget(e, in, 2000)
	if got.TargetKcal != 2000 {
		t.Errorf("TargetKcal = %d, want 2000", got.TargetKcal)
	}
	if got.BMR != e.BMR || got.TDEE != e.TDEE {
		t.Errorf("basal numbers changed: %+v", got)
	}
	if got.ProteinG != 84 {
		t.Errorf("ProteinG = %d, want 84 (per kg, unchanged)", got.ProteinG)
	}
	if !strings.Contains(got.Note, "自定值") {
		t.Errorf("Note = %q, want it to say where the target came from", got.Note)
	}
	// A zero or nonsense target leaves the estimate alone.
	if same := ApplyTarget(e, in, 0); same.TargetKcal != e.TargetKcal {
		t.Errorf("ApplyTarget(0) = %d, want %d", same.TargetKcal, e.TargetKcal)
	}
}

// The deficit floor keeps a small profile from getting a crash-diet number.
func TestEnergyTargetFloor(t *testing.T) {
	e := EnergyFor(EnergyInput{
		HeightCm: 150, WeightKg: 42, Age: 60, Sex: "female",
		ActivityLevel: "sedentary", Goals: []string{"减脂"},
	})
	if e.TargetKcal < minTargetKcal {
		t.Errorf("TargetKcal = %d, want at least %d", e.TargetKcal, minTargetKcal)
	}
}
