// Package ai talks to an OpenAI-compatible chat endpoint (DeepSeek by
// default) to plan a private-kitchen meal, and keeps the API key where the
// operating system keeps secrets.
//
// Two rules shape this package:
//
//   - The key never reaches the database, a backup or a log. It lives in the
//     system credential store through mygo.Secrets, and every error this
//     package returns is written to read like a person's sentence, never with
//     the request body or headers.
//   - The model's answer is untrusted. Its JSON is parsed tolerantly, then the
//     local sichu package validates it. A failed answer is asked again once;
//     after that the caller falls back to the offline planner. Nothing here
//     ever blocks the UI: callers run it on a goroutine.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/gij072858-cmd/foodbox-go/internal/assets"
	"github.com/gij072858-cmd/foodbox-go/internal/services/sichu"
)

// Defaults for the DeepSeek OpenAI-compatible endpoint.
const (
	DefaultBaseURL = "https://api.deepseek.com"
	DefaultModel   = "deepseek-chat"
	// SecretKey is the credential-store key. It is not a database key.
	SecretKey = "deepseek-api-key"
)

// DailyLimit is how many generations one day allows. The counter lives in
// app_meta and resets at local midnight.
const DailyLimit = 10

// KeyStore keeps the API key in the system credential store.
type KeyStore struct {
	secrets mygo.SecretsModule
	key     string
}

// NewKeyStore returns the store for the app's DeepSeek key.
func NewKeyStore() KeyStore {
	return KeyStore{secrets: mygo.Secrets, key: SecretKey}
}

// Get returns the stored key. A missing key is not an error.
func (k KeyStore) Get() (string, bool, error) {
	b, err := k.secrets.Get(k.key)
	if err != nil {
		if errors.Is(err, mygo.ErrSecretNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("读取本机凭据失败：%w", err)
	}
	key := strings.TrimSpace(string(b))
	if key == "" {
		return "", false, nil
	}
	return key, true, nil
}

// Set stores or replaces the key.
func (k KeyStore) Set(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("密钥不能为空")
	}
	if err := k.secrets.Set(k.key, []byte(key)); err != nil {
		return fmt.Errorf("保存密钥失败：%w", err)
	}
	return nil
}

// Delete removes the stored key. Deleting nothing is not an error.
func (k KeyStore) Delete() error {
	if err := k.secrets.Delete(k.key); err != nil {
		return fmt.Errorf("清除密钥失败：%w", err)
	}
	return nil
}

// Usage is one day's AI request count, stored as JSON in app_meta.
type Usage struct {
	Date string `json:"date"`
	Used int    `json:"used"`
}

// ParseUsage decodes the stored counter. An empty or corrupt value reads as a
// fresh day rather than blocking the feature.
func ParseUsage(raw string) Usage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Usage{}
	}
	var u Usage
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		return Usage{}
	}
	if u.Used < 0 {
		u.Used = 0
	}
	return u
}

// ForDay resets the counter when the stored day is not today.
func (u Usage) ForDay(today string) Usage {
	if u.Date != today {
		return Usage{Date: today}
	}
	return u
}

// Remaining is how many generations are left today.
func (u Usage) Remaining() int {
	n := DailyLimit - u.Used
	if n < 0 {
		return 0
	}
	return n
}

// Exhausted reports whether today's allowance is used up.
func (u Usage) Exhausted() bool { return u.Remaining() == 0 }

// After is the counter after one more generation.
func (u Usage) After() Usage {
	u.Used++
	return u
}

// JSON encodes the counter for app_meta.
func (u Usage) JSON() string {
	b, err := json.Marshal(u)
	if err != nil {
		return ""
	}
	return string(b)
}

// Today formats a time as the local date key the counter uses.
func Today(now time.Time) string { return now.Format("2006-01-02") }

// Message is one chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Provider calls one OpenAI-compatible endpoint.
type Provider struct {
	// BaseURL is injectable so tests can point at an httptest server.
	BaseURL string
	Model   string
	APIKey  string
	// System is the rules document; NewProvider fills it from assets.
	System string
	Client *http.Client
}

// NewProvider makes a provider for the DeepSeek endpoint using the embedded
// rules file as the system prompt.
func NewProvider(apiKey string) *Provider {
	system, _ := assets.Data("sichu_rules.md")
	return &Provider{
		BaseURL: DefaultBaseURL,
		Model:   DefaultModel,
		APIKey:  apiKey,
		System:  string(system),
		Client:  &http.Client{Timeout: 90 * time.Second},
	}
}

// endpoint is the chat completions URL.
func (p *Provider) endpoint() string {
	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	return base + "/chat/completions"
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Stream         bool            `json:"stream"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// chat sends one completion request and returns the reply text.
func (p *Provider) chat(ctx context.Context, messages []Message, maxTokens int, wantJSON bool) (string, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return "", errors.New("还没有配置 AI 密钥")
	}
	body := chatRequest{
		Model:       p.Model,
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   maxTokens,
	}
	if wantJSON {
		body.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(),
		bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("连接 AI 服务失败：%w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("读取 AI 回复失败：%w", err)
	}
	var parsed chatResponse
	decodeErr := json.Unmarshal(raw, &parsed)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decodeErr == nil && parsed.Error != nil && parsed.Error.Message != "" {
			return "", fmt.Errorf("AI 服务返回 %d：%s", resp.StatusCode, clip(parsed.Error.Message))
		}
		return "", fmt.Errorf("AI 服务返回 %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return "", fmt.Errorf("无法解析 AI 服务回复：%w", decodeErr)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("AI 服务报错：%s", clip(parsed.Error.Message))
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("AI 服务没有返回内容")
	}
	return parsed.Choices[0].Message.Content, nil
}

// Ping checks the key and the connection with a tiny request that does not
// count against the daily quota.
func (p *Provider) Ping(ctx context.Context) error {
	_, err := p.chat(ctx, []Message{
		{Role: "user", Content: "只回复两个字母 OK。"},
	}, 8, false)
	return err
}

// Generate asks the model for a plan, validates it locally, and asks once
// more when the answer is unusable. The returned plan is already enriched with
// library data and validated; an error means the caller should fall back to
// the offline planner.
func (p *Provider) Generate(ctx context.Context, in sichu.PlanInput) (sichu.MealPlan, error) {
	user := BuildUserPrompt(in)
	messages := []Message{
		{Role: "system", Content: p.System},
		{Role: "user", Content: user},
	}
	content, err := p.chat(ctx, messages, 4000, true)
	if err != nil {
		return sichu.MealPlan{}, err
	}
	plan, err := p.parseAndValidate(content, in)
	if err == nil {
		return plan, nil
	}
	firstErr := err

	// One correction round: show the model its own answer and the exact
	// reason it was rejected.
	messages = append(messages,
		Message{Role: "assistant", Content: content},
		Message{Role: "user", Content: correction(firstErr)},
	)
	content, err = p.chat(ctx, messages, 4000, true)
	if err != nil {
		return sichu.MealPlan{}, err
	}
	plan, err = p.parseAndValidate(content, in)
	if err != nil {
		return sichu.MealPlan{}, fmt.Errorf("AI 结果两次不合规：%w", err)
	}
	return plan, nil
}

// parseAndValidate parses one answer and runs the local contract on it.
func (p *Provider) parseAndValidate(content string, in sichu.PlanInput) (sichu.MealPlan, error) {
	plan, err := ParsePlan(content)
	if err != nil {
		return sichu.MealPlan{}, err
	}
	plan = sichu.Enrich(plan, in)
	if err := sichu.Validate(plan, in); err != nil {
		return sichu.MealPlan{}, err
	}
	return plan, nil
}

// correction is the user turn that asks for a fix.
func correction(err error) string {
	return "上一次输出不合格：" + clip(err.Error()) +
		"。请只输出一个 JSON 对象，遵守结构约束，recipeId 只能引用清单里的库内菜式，不要输出解释文字。"
}

// ParsePlan extracts a plan from a model reply. Markdown fences and prose
// around the JSON object are tolerated: models add them even when asked not
// to.
func ParsePlan(content string) (sichu.MealPlan, error) {
	body, err := extractJSON(content)
	if err != nil {
		return sichu.MealPlan{}, err
	}
	var wire wirePlan
	if err := json.Unmarshal([]byte(body), &wire); err != nil {
		return sichu.MealPlan{}, fmt.Errorf("AI 返回的 JSON 无法解析：%w", err)
	}
	plan := wire.toPlan()
	if len(plan.Meals) == 0 {
		return sichu.MealPlan{}, errors.New("AI 返回的 JSON 里没有餐次")
	}
	return plan, nil
}

// extractJSON finds the JSON object in a reply, tolerating code fences.
func extractJSON(content string) (string, error) {
	text := strings.TrimSpace(content)
	if text == "" {
		return "", errors.New("AI 返回了空内容")
	}
	if i := strings.Index(text, "```"); i >= 0 {
		rest := text[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "JSON")
		if j := strings.Index(rest, "```"); j >= 0 {
			text = strings.TrimSpace(rest[:j])
		} else {
			text = strings.TrimSpace(rest)
		}
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return "", errors.New("AI 返回里找不到 JSON 对象")
	}
	return text[start : end+1], nil
}

type wirePlan struct {
	Meals []wireMeal `json:"meals"`
	Note  string     `json:"note"`
}

type wireMeal struct {
	Title     string     `json:"title"`
	MealStyle string     `json:"mealStyle"`
	Dishes    []wireDish `json:"dishes"`
}

type wireDish struct {
	Name            string       `json:"name"`
	RecipeID        *int64       `json:"recipeId"`
	Brief           string       `json:"brief"`
	DurationMinutes float64      `json:"durationMinutes"`
	WaitMinutes     float64      `json:"waitMinutes"`
	Kcal            *float64     `json:"kcal"`
	ProteinG        *float64     `json:"proteinG"`
	ServeNote       string       `json:"serveNote"`
	Draft           *sichu.Draft `json:"draft"`
}

func (w wirePlan) toPlan() sichu.MealPlan {
	plan := sichu.MealPlan{Note: strings.TrimSpace(w.Note)}
	for _, m := range w.Meals {
		style := sichu.Style(strings.TrimSpace(m.MealStyle))
		if style != sichu.StyleOneBowl && style != sichu.StyleCustom {
			style = sichu.StyleCombo
		}
		meal := sichu.Meal{
			Title: strings.TrimSpace(m.Title),
			Style: style,
		}
		for _, d := range m.Dishes {
			dish := sichu.Dish{
				Name:        strings.TrimSpace(d.Name),
				Brief:       strings.TrimSpace(d.Brief),
				WaitMinutes: int(d.WaitMinutes),
				ServeNote:   strings.TrimSpace(d.ServeNote),
				Draft:       d.Draft,
			}
			if d.RecipeID != nil {
				dish.RecipeID = *d.RecipeID
			}
			if d.DurationMinutes > 0 {
				dish.DurationMinutes = int(d.DurationMinutes)
			}
			if d.Kcal != nil {
				dish.Kcal = int(*d.Kcal)
			}
			if d.ProteinG != nil {
				dish.ProteinG = int(*d.ProteinG)
			}
			if dish.Name == "" && dish.Draft != nil {
				dish.Name = strings.TrimSpace(dish.Draft.Name)
			}
			meal.Dishes = append(meal.Dishes, dish)
		}
		plan.Meals = append(plan.Meals, meal)
	}
	return plan
}

// clip shortens a message for one-line display without cutting a rune.
func clip(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	runes := []rune(s)
	if len(runes) <= 80 {
		return s
	}
	return string(runes[:80]) + "…"
}
