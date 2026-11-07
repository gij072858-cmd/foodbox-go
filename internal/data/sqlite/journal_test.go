package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// A day's diary is one row: saving the same date twice updates it instead of
// appending a second row.
func TestJournalIsUniquePerDay(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	first, err := store.SaveJournal(ctx, domain.CookJournal{
		Date: "2026-11-05", Note: "第一版", PhotoPaths: []string{"/tmp/a.jpg"},
		RecordIDs: []int64{1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 {
		t.Fatal("first save returned no id")
	}

	second, err := store.SaveJournal(ctx, domain.CookJournal{
		Date: "2026-11-05", Note: "改过", PhotoPaths: []string{"/tmp/b.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Errorf("same day got a new row: %d then %d", first.ID, second.ID)
	}
	if second.Note != "改过" {
		t.Errorf("note = %q", second.Note)
	}
	if len(second.PhotoPaths) != 1 || second.PhotoPaths[0] != "/tmp/b.jpg" {
		t.Errorf("photos = %v", second.PhotoPaths)
	}

	all, err := store.ListJournals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("journals = %d rows, want 1", len(all))
	}
	if all[0].Date != "2026-11-05" || all[0].Note != "改过" {
		t.Errorf("stored journal = %+v", all[0])
	}

	got, ok, err := store.GetJournal(ctx, "2026-11-05")
	if err != nil || !ok || got.Note != "改过" {
		t.Errorf("GetJournal = %+v, ok %v, err %v", got, ok, err)
	}
	if _, ok, err := store.GetJournal(ctx, "2026-11-06"); err != nil || ok {
		t.Errorf("missing day = ok %v, err %v", ok, err)
	}
}

func TestProfileListsRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	p, err := store.GetProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Nickname != "食匣用户" {
		t.Errorf("default nickname = %q", p.Nickname)
	}

	if err := store.SaveProfile(ctx, domain.Profile{
		ID: 1, Nickname: "小满", AvatarPath: "/tmp/avatar.png",
		FavoriteRecipes: []int64{3, 7}, WantedRecipes: []int64{9},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Nickname != "小满" || got.AvatarPath != "/tmp/avatar.png" {
		t.Errorf("profile = %+v", got)
	}
	if len(got.FavoriteRecipes) != 2 || got.FavoriteRecipes[1] != 7 {
		t.Errorf("favorites = %v", got.FavoriteRecipes)
	}
	if len(got.WantedRecipes) != 1 || got.WantedRecipes[0] != 9 {
		t.Errorf("wanted = %v", got.WantedRecipes)
	}

	if err := store.SetFavoriteRecipeIDs(ctx, []int64{11}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWantedRecipeIDs(ctx, []int64{}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.FavoriteRecipes) != 1 || got.FavoriteRecipes[0] != 11 {
		t.Errorf("favorites after set = %v", got.FavoriteRecipes)
	}
	if len(got.WantedRecipes) != 0 {
		t.Errorf("wanted after set = %v", got.WantedRecipes)
	}
}

// The first-run questionnaire writes user_profiles and the private kitchen's
// initial restrictions in one transaction, and onboarding is recorded.
func TestSaveOnboardingWritesBothProfiles(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := store.GetUserProfile(ctx); err != nil {
		t.Fatal(err)
	}
	up, err := store.GetUserProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if up.Onboarded {
		t.Error("fresh database reports onboarded")
	}

	if err := store.SaveOnboarding(ctx, domain.UserProfile{
		StoveCount: 3, Cookware: `{"wok":true}`, Hometown: "四川",
		AgeRange: "26-35", Spicy: "中辣", Saltiness: "适中",
		Restrictions: "不吃香菜", AllergyNote: "花生",
		HouseholdSize: 3, HouseholdMix: `{"adult":2,"kid":1,"elder":0}`,
		Onboarded: true,
	}); err != nil {
		t.Fatal(err)
	}

	up, err = store.GetUserProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !up.Onboarded || up.StoveCount != 3 || up.Hometown != "四川" {
		t.Errorf("user profile = %+v", up)
	}
	if up.Restrictions != "不吃香菜" || up.HouseholdSize != 3 {
		t.Errorf("user profile = %+v", up)
	}

	rows, err := store.Query(ctx, `SELECT restrictions, allergy_note FROM sichu_profiles WHERE id = 1`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) != 1 {
		t.Fatalf("sichu_profiles rows = %d", len(rows.Values))
	}
	if got := asString(rows.Values[0][0]); got != "不吃香菜" {
		t.Errorf("sichu restrictions = %q", got)
	}
}

func TestRecipeBriefsAndClearCookedRecords(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Execute(ctx, `INSERT INTO recipes (id, name, cuisine, region, main_category)
		VALUES (1, '青椒肉丝', '家常菜', '川菜', '猪肉')`); err != nil {
		t.Fatal(err)
	}
	briefs, err := store.RecipeBriefs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(briefs) != 1 || briefs[0].Name != "青椒肉丝" || briefs[0].Cuisine != "家常菜" {
		t.Errorf("briefs = %+v", briefs)
	}

	if err := store.SettleRun(ctx, []int64{1}, time.Now(), nil, ""); err != nil {
		t.Fatal(err)
	}
	if n, err := store.RecordCount(ctx); err != nil || n != 1 {
		t.Fatalf("records = %d, err %v", n, err)
	}
	if err := store.ClearCookedRecords(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := store.RecordCount(ctx); err != nil || n != 0 {
		t.Errorf("records after clear = %d, err %v", n, err)
	}
}
