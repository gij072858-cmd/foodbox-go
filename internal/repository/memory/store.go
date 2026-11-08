// Package memory is an in-process Repository for tests. It holds the same
// values the SQLite store would return, without a database, a migration or
// the seed, so an app test that only needs persistence does not pay for
// three seconds of JSON import.
//
// It is not a second implementation of the app's rules: settlement,
// backup and seeding stay with the real store, and those tests keep one.
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// ErrUnsupported marks the two raw doors only the real store opens.
var ErrUnsupported = errors.New("memory store: not supported")

// Store is a Repository in memory. The zero value is not ready; use New.
type Store struct {
	mu sync.Mutex

	recipes map[int64]domain.Recipe
	steps   map[int64][]domain.Step

	items  []domain.Item
	nextID int64

	schedule []domain.ScheduleItem
	run      domain.RunSession
	runSet   bool

	records []domain.Record

	journals map[string]domain.CookJournal

	profile      domain.Profile
	userProfile  domain.UserProfile
	sichu        domain.SichuProfile
	sichuHasRow  bool
	members      []domain.MemberProfile
	nextMemberID int64
	aiUsage      string
	meta         map[string]any
}

// New returns an empty store.
func New() *Store {
	return &Store{
		recipes:  map[int64]domain.Recipe{},
		steps:    map[int64][]domain.Step{},
		journals: map[string]domain.CookJournal{},
		meta:     map[string]any{},
		nextID:   1,
		sichu:    domain.SichuProfile{ExploreRatio: 0.3},
	}
}

var _ interface {
	Close() error
} = (*Store)(nil)

// Migrate is a no-op: there is no schema.
func (s *Store) Migrate(context.Context) error { return nil }

// Close drops the data.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	*s = *New()
	return nil
}

// Query and Run are the raw SQL doors. A test that needs them wants the
// real store, not this one.
func (s *Store) Query(context.Context, string, ...any) (sqlite.QueryResult, error) {
	return sqlite.QueryResult{}, ErrUnsupported
}

func (s *Store) Run(context.Context, []sqlite.Statement) error { return ErrUnsupported }

// SeedRecipes installs a recipe snapshot, as the seed import would.
func (s *Store) SeedRecipes(recipes []domain.Recipe, steps map[int64][]domain.Step) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range recipes {
		s.recipes[r.ID] = r
	}
	for id, list := range steps {
		s.steps[id] = list
	}
}

// Reader.

func (s *Store) ListAll(context.Context) ([]domain.Recipe, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Recipe, 0, len(s.recipes))
	for _, r := range s.recipes {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) AllSteps(context.Context) (map[int64][]domain.Step, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int64][]domain.Step, len(s.steps))
	for id, list := range s.steps {
		out[id] = append([]domain.Step{}, list...)
	}
	return out, nil
}

func (s *Store) RecipeBriefs(context.Context) ([]domain.RecipeBrief, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.RecipeBrief, 0, len(s.recipes))
	for _, r := range s.recipes {
		out = append(out, domain.RecipeBrief{
			ID: r.ID, Name: r.Name, Cuisine: r.Cuisine, Region: r.Region,
			MainCategory: r.MainCategory,
		})
	}
	return out, nil
}

func (s *Store) ListItems(context.Context) ([]domain.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Item{}, s.items...), nil
}

func (s *Store) ListSchedule(context.Context) ([]domain.ScheduleItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.ScheduleItem{}, s.schedule...), nil
}

func (s *Store) LoadRunSession(context.Context) (domain.RunSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run, s.runSet, nil
}

func (s *Store) ListRecords(_ context.Context, limit int) ([]domain.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]domain.Record{}, s.records...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CompletedAt.After(out[j].CompletedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) RecordCount(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records), nil
}

func (s *Store) MealDayCount(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	days := map[string]bool{}
	for _, r := range s.records {
		days[r.CompletedAt.In(time.Local).Format("2006-01-02")] = true
	}
	return len(days), nil
}

func (s *Store) ListJournals(context.Context) ([]domain.CookJournal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.CookJournal, 0, len(s.journals))
	for _, j := range s.journals {
		out = append(out, j)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

func (s *Store) GetProfile(context.Context) (domain.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profile, nil
}

func (s *Store) GetUserProfile(context.Context) (domain.UserProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.userProfile, nil
}

func (s *Store) GetSichuProfile(context.Context) (domain.SichuProfile, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sichu, s.sichuHasRow, nil
}

func (s *Store) ListMembers(context.Context) ([]domain.MemberProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.MemberProfile{}, s.members...), nil
}

func (s *Store) MetaGetJSON(_ context.Context, key string, dst any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.meta[key]
	if !ok {
		return false, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(raw, dst)
}

func (s *Store) GetAIUsageJSON(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aiUsage, nil
}

// Writer.

func (s *Store) CreateRecipe(_ context.Context, rec domain.Recipe) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.ID == 0 {
		rec.ID = s.nextRecipeID()
	}
	s.recipes[rec.ID] = rec
	return rec.ID, nil
}

func (s *Store) nextRecipeID() int64 {
	var max int64
	for id := range s.recipes {
		if id > max {
			max = id
		}
	}
	return max + 1
}

func (s *Store) UpdateRecipe(_ context.Context, rec domain.Recipe) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.recipes[rec.ID]; !ok {
		return fmt.Errorf("recipe %d: not found", rec.ID)
	}
	s.recipes[rec.ID] = rec
	return nil
}

func (s *Store) DeleteRecipe(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.recipes, id)
	delete(s.steps, id)
	return nil
}

func (s *Store) CreateItem(_ context.Context, it domain.Item) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it.ID == 0 {
		it.ID = s.nextID
		s.nextID++
	}
	s.items = append(s.items, it)
	return it.ID, nil
}

func (s *Store) CreateItems(_ context.Context, items []domain.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range items {
		if it.ID == 0 {
			it.ID = s.nextID
			s.nextID++
		}
		s.items = append(s.items, it)
	}
	return nil
}

func (s *Store) UpdateItem(_ context.Context, it domain.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == it.ID {
			s.items[i] = it
			return nil
		}
	}
	return fmt.Errorf("item %d: not found", it.ID)
}

func (s *Store) DeleteItem(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *Store) SetItemDepleted(_ context.Context, id int64, depleted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].IsDepleted = depleted
			return nil
		}
	}
	return fmt.Errorf("item %d: not found", id)
}

func (s *Store) AddScheduleItem(_ context.Context, recipeID int64, servings int, meal string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.schedule {
		if it.RecipeID == recipeID {
			return false, nil
		}
	}
	s.schedule = append(s.schedule, domain.ScheduleItem{
		RecipeID: recipeID, AddedAt: time.Now(), Servings: servings,
		Status: domain.SchedulePending, Meal: meal,
	})
	return true, nil
}

func (s *Store) RemoveScheduleItem(_ context.Context, recipeID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.schedule {
		if s.schedule[i].RecipeID == recipeID {
			s.schedule = append(s.schedule[:i], s.schedule[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *Store) EnqueueRecipes(_ context.Context, items []domain.ScheduleItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		found := false
		for _, it := range s.schedule {
			if it.RecipeID == item.RecipeID {
				found = true
				break
			}
		}
		if !found {
			item.AddedAt = time.Now()
			if item.Status == "" {
				item.Status = domain.SchedulePending
			}
			s.schedule = append(s.schedule, item)
		}
	}
	return nil
}

func (s *Store) SaveRunSession(_ context.Context, session domain.RunSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run, s.runSet = session, true
	return nil
}

func (s *Store) ClearRunSession(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run, s.runSet = domain.RunSession{}, false
	return nil
}

func (s *Store) SettleRunFull(_ context.Context, recipeIDs []int64, completedAt time.Time, sessionSeconds *int, note string, _ bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	settled := make(map[int64]bool, len(recipeIDs))
	for _, id := range recipeIDs {
		settled[id] = true
		s.records = append(s.records, domain.Record{
			ID: int64(len(s.records) + 1), RecipeID: id, CompletedAt: completedAt,
			SessionSeconds: sessionSeconds, Note: note, XPGained: 10,
		})
	}
	// Mirror the SQLite transaction: ten XP per dish and one meal, so tests
	// that settle through this store see the same level change.
	s.profile.XP += 10 * len(recipeIDs)
	s.profile.TotalMeals++
	kept := s.schedule[:0]
	for _, it := range s.schedule {
		if !settled[it.RecipeID] {
			kept = append(kept, it)
		}
	}
	s.schedule = kept
	s.run, s.runSet = domain.RunSession{}, false
	return nil
}

func (s *Store) SaveJournal(_ context.Context, j domain.CookJournal) (domain.CookJournal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.Date == "" {
		return j, errors.New("journal: empty date")
	}
	s.journals[j.Date] = j
	return j, nil
}

func (s *Store) ClearCookedRecords(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
	return nil
}

func (s *Store) SaveProfile(_ context.Context, p domain.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profile = p
	return nil
}

func (s *Store) SaveUserProfile(_ context.Context, p domain.UserProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userProfile = p
	return nil
}

func (s *Store) SaveOnboarding(_ context.Context, p domain.UserProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userProfile = p
	return nil
}

func (s *Store) SetFavoriteRecipeIDs(_ context.Context, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profile.FavoriteRecipes = append([]int64{}, ids...)
	return nil
}

func (s *Store) SetWantedRecipeIDs(_ context.Context, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profile.WantedRecipes = append([]int64{}, ids...)
	return nil
}

func (s *Store) SaveSichuProfileWithSelf(_ context.Context, p domain.SichuProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sichu, s.sichuHasRow = p, true
	self := domain.MemberProfile{
		Name: "本人", Role: "adult", IsSelf: true,
		HeightCm: p.HeightCm, WeightKg: p.WeightKg, Age: p.Age,
	}
	// The self member mirrors the profile, as the real store's single
	// transaction does.
	for i := range s.members {
		if s.members[i].IsSelf {
			self.ID = s.members[i].ID
			s.members[i] = self
			return nil
		}
	}
	self.ID = s.nextMemberID + 1
	s.nextMemberID++
	s.members = append([]domain.MemberProfile{self}, s.members...)
	return nil
}

func (s *Store) CreateMember(_ context.Context, m domain.MemberProfile) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextMemberID++
	m.ID = s.nextMemberID
	s.members = append(s.members, m)
	return m.ID, nil
}

func (s *Store) UpdateMember(_ context.Context, m domain.MemberProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.members {
		if s.members[i].ID == m.ID {
			s.members[i] = m
			return nil
		}
	}
	return fmt.Errorf("member %d: not found", m.ID)
}

func (s *Store) DeleteMember(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.members {
		if s.members[i].ID == id {
			s.members = append(s.members[:i], s.members[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *Store) SaveAIUsageJSON(_ context.Context, raw string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aiUsage = raw
	return nil
}

func (s *Store) ClearAIUsage(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aiUsage = ""
	return nil
}

func (s *Store) MetaSetJSON(_ context.Context, key string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meta[key] = v
	return nil
}

// The compile-time proof that the test store is the app's store.
var _ interface {
	Migrate(context.Context) error
	Close() error
	Query(context.Context, string, ...any) (sqlite.QueryResult, error)
	Run(context.Context, []sqlite.Statement) error
	ListAll(context.Context) ([]domain.Recipe, error)
} = (*Store)(nil)

// MetaGet reads a preference blob back as JSON, for the tests that assert
// what a restart would load.
func (s *Store) MetaGet(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.meta[key]
	if !ok {
		return "", nil
	}
	raw, err := json.Marshal(v)
	return string(raw), err
}
