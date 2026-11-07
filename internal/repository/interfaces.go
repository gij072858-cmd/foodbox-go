package repository

// The app talks to three narrow interfaces instead of the SQLite store: it
// reads, it writes, and bootstrap migrates and closes. The concrete store
// already has every method, so App.store swaps type without a call site
// changing. Interfaces list what App calls, nothing else.

import (
	"context"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// Reader is every query the app makes.
type Reader interface {
	// Recipes.
	ListAll(ctx context.Context) ([]domain.Recipe, error)
	AllSteps(ctx context.Context) (map[int64][]domain.Step, error)
	RecipeBriefs(ctx context.Context) ([]domain.RecipeBrief, error)
	// Pantry.
	ListItems(ctx context.Context) ([]domain.Item, error)
	// Schedule and run session.
	ListSchedule(ctx context.Context) ([]domain.ScheduleItem, error)
	LoadRunSession(ctx context.Context) (domain.RunSession, bool, error)
	// History and journals.
	ListRecords(ctx context.Context, limit int) ([]domain.Record, error)
	RecordCount(ctx context.Context) (int, error)
	MealDayCount(ctx context.Context) (int, error)
	ListJournals(ctx context.Context) ([]domain.CookJournal, error)
	// Profiles.
	GetProfile(ctx context.Context) (domain.Profile, error)
	GetUserProfile(ctx context.Context) (domain.UserProfile, error)
	GetSichuProfile(ctx context.Context) (domain.SichuProfile, bool, error)
	ListMembers(ctx context.Context) ([]domain.MemberProfile, error)
	// App metadata and AI quota.
	MetaGetJSON(ctx context.Context, key string, dst any) (bool, error)
	GetAIUsageJSON(ctx context.Context) (string, error)
}

// Writer is every change the app makes. Saving a recipe replaces its steps
// in one transaction; settling a run deducts the pantry and writes the
// history in one transaction too.
type Writer interface {
	// Recipes.
	CreateRecipe(ctx context.Context, rec domain.Recipe) (int64, error)
	UpdateRecipe(ctx context.Context, rec domain.Recipe) error
	DeleteRecipe(ctx context.Context, id int64) error
	// Pantry.
	CreateItem(ctx context.Context, it domain.Item) (int64, error)
	CreateItems(ctx context.Context, items []domain.Item) error
	UpdateItem(ctx context.Context, it domain.Item) error
	DeleteItem(ctx context.Context, id int64) error
	SetItemDepleted(ctx context.Context, id int64, depleted bool) error
	// Schedule.
	AddScheduleItem(ctx context.Context, recipeID int64, servings int, meal string) (bool, error)
	RemoveScheduleItem(ctx context.Context, recipeID int64) error
	EnqueueRecipes(ctx context.Context, items []domain.ScheduleItem) error
	// Run session.
	SaveRunSession(ctx context.Context, session domain.RunSession) error
	ClearRunSession(ctx context.Context) error
	SettleRunFull(ctx context.Context, recipeIDs []int64, completedAt time.Time, sessionSeconds *int, note string, deductPantry bool) error
	// Journals and history.
	SaveJournal(ctx context.Context, j domain.CookJournal) (domain.CookJournal, error)
	ClearCookedRecords(ctx context.Context) error
	// Profiles.
	SaveProfile(ctx context.Context, p domain.Profile) error
	SaveUserProfile(ctx context.Context, p domain.UserProfile) error
	SaveOnboarding(ctx context.Context, p domain.UserProfile) error
	SetFavoriteRecipeIDs(ctx context.Context, ids []int64) error
	SetWantedRecipeIDs(ctx context.Context, ids []int64) error
	SaveSichuProfileWithSelf(ctx context.Context, p domain.SichuProfile) error
	CreateMember(ctx context.Context, m domain.MemberProfile) (int64, error)
	UpdateMember(ctx context.Context, m domain.MemberProfile) error
	DeleteMember(ctx context.Context, id int64) error
	// AI usage and app metadata.
	SaveAIUsageJSON(ctx context.Context, raw string) error
	ClearAIUsage(ctx context.Context) error
	MetaSetJSON(ctx context.Context, key string, v any) error
}

// Admin is the lifecycle and the two raw doors backup and seeding need:
// migrate once at startup, close at quit, read a table for a backup and
// write a batch in one transaction when restoring or seeding.
type Admin interface {
	Migrate(ctx context.Context) error
	Close() error
	Query(ctx context.Context, sql string, args ...any) (sqlite.QueryResult, error)
	Run(ctx context.Context, stmts []sqlite.Statement) error
}

// Repository is what App holds.
type Repository interface {
	Reader
	Writer
	Admin
}

// The SQLite store is the production implementation; this line is the
// compile-time proof that a store can be handed to App unchanged.
var _ Repository = (*sqlite.Store)(nil)
