package memory_test

import (
	"context"
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
	"github.com/gij072858-cmd/foodbox-go/internal/repository/memory"
)

// The test store is the app's store: this line fails to compile if a method
// is added to Repository and not here.
var _ repository.Repository = memory.New()

func TestStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := memory.New()

	id, err := s.CreateItem(ctx, domain.Item{Name: "番茄"})
	if err != nil || id == 0 {
		t.Fatalf("create item: id=%d err=%v", id, err)
	}
	if err := s.SetItemDepleted(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	items, err := s.ListItems(ctx)
	if err != nil || len(items) != 1 || !items[0].IsDepleted {
		t.Fatalf("items = %+v, %v", items, err)
	}

	if ok, _ := s.AddScheduleItem(ctx, 7, 2, ""); !ok {
		t.Fatal("first add refused")
	}
	if ok, _ := s.AddScheduleItem(ctx, 7, 2, ""); ok {
		t.Fatal("duplicate add accepted")
	}
	if queue, _ := s.ListSchedule(ctx); len(queue) != 1 {
		t.Fatalf("queue = %+v", queue)
	}

	if _, err := s.SaveJournal(ctx, domain.CookJournal{Date: "2026-11-06", Note: "少盐"}); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListJournals(ctx); len(list) != 1 || list[0].Note != "少盐" {
		t.Fatalf("journals = %+v", list)
	}

	if err := s.SaveSichuProfileWithSelf(ctx, domain.SichuProfile{HeightCm: 175, Allergens: "peanut"}); err != nil {
		t.Fatal(err)
	}
	profile, ok, err := s.GetSichuProfile(ctx)
	if err != nil || !ok || profile.Allergens != "peanut" {
		t.Fatalf("profile = %+v ok=%v err=%v", profile, ok, err)
	}
	members, _ := s.ListMembers(ctx)
	if len(members) != 1 || !members[0].IsSelf {
		t.Fatalf("members = %+v", members)
	}
}
