package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// The body profile and the self member are one person: saving the profile
// writes both rows in one transaction.
func TestSichuProfileSyncsSelfMember(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.GetSichuProfile(ctx); err != nil {
		t.Fatal(err)
	}
	members, err := store.ListMembers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || !members[0].IsSelf {
		t.Fatalf("seeded members = %+v", members)
	}

	profile := domain.SichuProfile{
		HeightCm: 175, WeightKg: 70, Age: 31, Sex: "male",
		ActivityLevel: "moderate", TargetWeightKg: 66, BodyFatPct: 18.5,
		LifeStage: "none", Allergens: "peanut", HealthTags: "gout",
		HealthStages: `{"gout":"goutRemission"}`, HealthNote: "少吃海鲜",
		NutritionGoal: "fatLoss", NutritionGoals: `["fatLoss"]`,
		CalorieTarget: 1800, Restrictions: "halal", AllergyNote: "花生",
		ExploreRatio: 0.4, TargetMeals: "lunch,dinner",
	}
	if err := store.SaveSichuProfileWithSelf(ctx, profile); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.GetSichuProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("saved profile reads back as missing")
	}
	if got.HeightCm != 175 || got.WeightKg != 70 || got.HealthTags != "gout" ||
		got.ExploreRatio != 0.4 || got.Restrictions != "halal" || got.Allergens != "peanut" {
		t.Fatalf("profile round-trip = %+v", got)
	}

	members, err = store.ListMembers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Fatalf("self member duplicated: %+v", members)
	}
	self := members[0]
	if !self.IsSelf || self.HeightCm != 175 || self.WeightKg != 70 ||
		self.HealthTags != "gout" || self.Allergens != "peanut" ||
		self.NutritionGoals != `["fatLoss"]` {
		t.Fatalf("self member = %+v", self)
	}

	// Saving again must update the same row, not add another self card.
	profile.WeightKg = 69
	if err := store.SaveSichuProfileWithSelf(ctx, profile); err != nil {
		t.Fatal(err)
	}
	members, err = store.ListMembers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].WeightKg != 69 {
		t.Fatalf("after second save: %+v", members)
	}
}

// Members other than self can be created, edited and deleted; the self card
// refuses both edits and deletes because the profile page owns it.
func TestMembersCRUD(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	id, err := store.CreateMember(ctx, domain.MemberProfile{
		Name: "小满", Role: "kid", Age: 8, HealthTags: "gout",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("no id returned")
	}

	members, err := store.ListMembers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %+v", members)
	}
	if !members[0].IsSelf || members[1].Name != "小满" {
		t.Fatalf("order = %+v", members)
	}

	members[1].WeightKg = 25
	if err := store.UpdateMember(ctx, members[1]); err != nil {
		t.Fatal(err)
	}
	members, err = store.ListMembers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if members[1].WeightKg != 25 {
		t.Fatalf("weight not saved: %+v", members[1])
	}

	self := members[0]
	self.HeightCm = 180
	if err := store.UpdateMember(ctx, self); !errors.Is(err, ErrSelfMember) {
		t.Fatalf("self update error = %v", err)
	}
	if err := store.DeleteMember(ctx, self.ID); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("self delete error = %v", err)
	}
	if _, err := store.CreateMember(ctx, domain.MemberProfile{Name: "本人", IsSelf: true}); !errors.Is(err, ErrSelfMember) {
		t.Fatalf("self create error = %v", err)
	}

	if err := store.DeleteMember(ctx, id); err != nil {
		t.Fatal(err)
	}
	if members, _ := store.ListMembers(ctx); len(members) != 1 {
		t.Fatalf("after delete = %+v", members)
	}
}

// The AI counter lives in app_meta, holds no key, and can be cleared.
func TestAIUsage(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := store.GetAIUsageJSON(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "" {
		t.Fatalf("fresh usage = %q", raw)
	}
	if err := store.SaveAIUsageJSON(ctx, `{"date":"2026-11-05","used":3}`); err != nil {
		t.Fatal(err)
	}
	raw, err = store.GetAIUsageJSON(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" || raw[0] != '{' {
		t.Fatalf("stored usage = %q", raw)
	}
	if err := store.ClearAIUsage(ctx); err != nil {
		t.Fatal(err)
	}
	if raw, _ := store.GetAIUsageJSON(ctx); raw != "" {
		t.Fatalf("usage after clear = %q", raw)
	}
}
