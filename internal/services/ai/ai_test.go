package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/services/sichu"
)

func minutes(v float64) *float64 { return &v }

func aiRecipes() []domain.Recipe {
	mk := func(id int64, name, cuisine, category string, ings ...string) domain.Recipe {
		lines := make([]domain.Ingredient, 0, len(ings))
		for _, in := range ings {
			lines = append(lines, domain.Ingredient{Name: in, Amount: 100, Unit: "g", Required: true})
		}
		return domain.Recipe{
			ID: id, Name: name, Cuisine: cuisine, MainCategory: category,
			DurationMinutes: minutes(20), BaseServings: 2, IsDish: true,
			VersionGroup: name, Ingredients: lines,
		}
	}
	return []domain.Recipe{
		mk(1, "青椒肉丝", "家常菜", "猪肉", "猪肉", "青椒"),
		mk(2, "香煎鸡胸", "快手菜", "鸡肉", "鸡胸肉"),
		mk(3, "红烧肉", "炖菜", "猪肉", "五花肉"),
		mk(4, "蒜蓉西兰花", "快手菜", "蔬菜", "西兰花"),
		mk(5, "清炒时蔬", "家常菜", "蔬菜", "时蔬"),
		mk(6, "番茄蛋花汤", "汤羹", "蛋类", "番茄", "鸡蛋"),
		mk(7, "紫菜蛋花汤", "汤羹", "蛋类", "紫菜", "鸡蛋"),
		mk(8, "黄金蛋炒饭", "快手菜", "主食", "米饭", "鸡蛋"),
	}
}

func aiInput() sichu.PlanInput {
	return sichu.PlanInput{
		Recipes:  aiRecipes(),
		Now:      time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC),
		Style:    sichu.StyleCombo,
		Meals:    []string{"午餐"},
		Servings: 2,
		Profile:  domain.SichuProfile{ExploreRatio: 0.3},
	}
}

const validAnswer = `{
  "meals": [{
    "title": "午餐",
    "mealStyle": "combo",
    "dishes": [
      {"name": "青椒肉丝", "recipeId": 1, "brief": "家常下饭", "durationMinutes": 20},
      {"name": "蒜蓉西兰花", "recipeId": 4, "brief": "清爽蔬菜", "durationMinutes": 10},
      {"name": "番茄蛋花汤", "recipeId": 6, "brief": "先喝两口", "durationMinutes": 10}
    ]
  }],
  "note": "按档案配的。"
}`

func TestParsePlanFencedJSON(t *testing.T) {
	content := "好的，方案如下：\n```json\n" + validAnswer + "\n```\n希望你喜欢。"
	plan, err := ParsePlan(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Meals) != 1 || len(plan.Meals[0].Dishes) != 3 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Meals[0].Dishes[0].RecipeID != 1 {
		t.Errorf("recipe id = %d", plan.Meals[0].Dishes[0].RecipeID)
	}
	if plan.Note == "" {
		t.Error("note not parsed")
	}
}

func TestParsePlanRejectsGarbage(t *testing.T) {
	for _, content := range []string{"", "我觉得今天吃点好的", `{"meals": []}`} {
		if _, err := ParsePlan(content); err == nil {
			t.Errorf("expected an error for %q", content)
		}
	}
}

func TestParsePlanDraft(t *testing.T) {
	content := `{"meals":[{"title":"晚餐","mealStyle":"oneBowl","dishes":[
	  {"name":"蒜蓉秋葵盖饭","recipeId":null,"brief":"新花样","draft":{
	    "name":"蒜蓉秋葵盖饭","cuisine":"家常菜","mainCategory":"蔬菜",
	    "ingredients":[{"name":"秋葵","amount":300,"unit":"g"}],
	    "steps":[{"instruction":"焯水后快炒"}]}}]}]}`
	plan, err := ParsePlan(content)
	if err != nil {
		t.Fatal(err)
	}
	dish := plan.Meals[0].Dishes[0]
	if dish.Draft == nil {
		t.Fatal("draft not parsed")
	}
	if dish.Draft.Ingredients[0].Name != "秋葵" {
		t.Errorf("draft ingredients = %+v", dish.Draft.Ingredients)
	}
	if len(dish.Draft.Steps) != 1 {
		t.Errorf("draft steps = %+v", dish.Draft.Steps)
	}
}

func TestUsageQuota(t *testing.T) {
	today := "2026-11-05"
	u := ParseUsage("")
	if u.Remaining() != DailyLimit {
		t.Fatalf("fresh remaining = %d", u.Remaining())
	}
	u = u.ForDay(today)
	for i := 0; i < DailyLimit; i++ {
		if u.Exhausted() {
			t.Fatalf("exhausted early at %d", i)
		}
		u = u.After()
	}
	if !u.Exhausted() {
		t.Fatal("quota not exhausted")
	}
	// A new local day resets the counter.
	if got := u.ForDay("2026-11-06"); got.Used != 0 || got.Remaining() != DailyLimit {
		t.Fatalf("reset = %+v", got)
	}
	// JSON round-trip through app_meta.
	back := ParseUsage(u.JSON())
	if back.Date != today || back.Used != DailyLimit {
		t.Fatalf("round-trip = %+v", back)
	}
	if got := ParseUsage("{broken"); got.Used != 0 {
		t.Fatalf("corrupt usage = %+v", got)
	}
}

// fakeServer answers chat completions with the given replies in order.
func fakeServer(t *testing.T, replies []string, capture *[]string) *httptest.Server {
	t.Helper()
	i := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("request body: %v", err)
		}
		if capture != nil {
			encoded, _ := json.Marshal(body)
			*capture = append(*capture, string(encoded))
		}
		reply := `{"error":{"message":"没有更多回复"}}`
		if i < len(replies) {
			reply = replies[i]
		}
		i++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func chatReply(content string) string {
	b, _ := json.Marshal(content)
	return `{"choices":[{"message":{"content":` + string(b) + `}}]}`
}

func testProvider(url string) *Provider {
	p := NewProvider("test-key")
	p.BaseURL = url
	return p
}

func TestProviderGenerate(t *testing.T) {
	var requests []string
	srv := fakeServer(t, []string{chatReply(validAnswer)}, &requests)
	p := testProvider(srv.URL)

	plan, err := p.Generate(context.Background(), aiInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Meals) != 1 || len(plan.Meals[0].Dishes) != 3 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Meals[0].Dishes[2].Name != "番茄蛋花汤" {
		t.Errorf("dish name = %q", plan.Meals[0].Dishes[2].Name)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %d", len(requests))
	}
	if !strings.Contains(requests[0], `"response_format":{"type":"json_object"}`) {
		t.Error("request did not ask for a JSON object")
	}
	if !strings.Contains(requests[0], "可引用的库内菜式") {
		t.Error("prompt missing the candidate list")
	}
}

// A structurally invalid answer is sent back once with the reason; the second
// answer wins.
func TestProviderRetriesOnce(t *testing.T) {
	bad := `{"meals":[{"title":"午餐","dishes":[{"name":"青椒肉丝","recipeId":1}]}]}`
	var requests []string
	srv := fakeServer(t, []string{chatReply(bad), chatReply(validAnswer)}, &requests)
	p := testProvider(srv.URL)

	plan, err := p.Generate(context.Background(), aiInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Meals[0].Dishes) != 3 {
		t.Fatalf("plan = %+v", plan)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if !strings.Contains(requests[1], "上一次输出不合格") {
		t.Error("retry did not include the correction")
	}
}

// Two bad answers in a row fail, so the caller can fall back locally.
func TestProviderFailsAfterRetry(t *testing.T) {
	var requests []string
	srv := fakeServer(t, []string{chatReply("不是 JSON"), chatReply("还是不是")}, &requests)
	p := testProvider(srv.URL)
	if _, err := p.Generate(context.Background(), aiInput()); err == nil {
		t.Fatal("expected an error after two bad answers")
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
}

func TestProviderServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"rate limited"}}`)
	}))
	defer srv.Close()
	p := testProvider(srv.URL)
	_, err := p.Generate(context.Background(), aiInput())
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("error = %v", err)
	}
}

func TestProviderWithoutKey(t *testing.T) {
	p := NewProvider("")
	p.BaseURL = "http://127.0.0.1:1"
	_, err := p.Generate(context.Background(), aiInput())
	if err == nil || !strings.Contains(err.Error(), "密钥") {
		t.Fatalf("error = %v", err)
	}
}

// A recipe id outside the candidate list is rejected by the local contract.
func TestProviderValidatesRecipeIDs(t *testing.T) {
	bad := `{"meals":[{"title":"午餐","dishes":[
	  {"name":"不存在的菜","recipeId":999},
	  {"name":"蒜蓉西兰花","recipeId":4},
	  {"name":"番茄蛋花汤","recipeId":6}]}]}`
	var requests []string
	srv := fakeServer(t, []string{chatReply(bad), chatReply(bad)}, &requests)
	p := testProvider(srv.URL)
	_, err := p.Generate(context.Background(), aiInput())
	if err == nil || !strings.Contains(err.Error(), "不在菜式库") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildUserPrompt(t *testing.T) {
	in := aiInput()
	in.Profile = domain.SichuProfile{
		HeightCm: 175, WeightKg: 70, Age: 30, Sex: "male",
		ActivityLevel: "moderate", HealthTags: "gout",
		Restrictions: "halal", ExploreRatio: 0.4, NutritionGoals: `["fatLoss"]`,
	}
	prompt := BuildUserPrompt(in)
	for _, want := range []string{
		"两菜一汤", "午餐", "2 人", "清真", "痛风", "减脂",
		"可引用的库内菜式", "2 | 香煎鸡胸", "不要给 draft",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "test-key") {
		t.Error("prompt leaked the key")
	}
	// A halal gout diner must not be offered pork.
	if strings.Contains(prompt, "3 | 红烧肉") {
		t.Error("prompt listed a dish that breaks the hard limits")
	}
}
