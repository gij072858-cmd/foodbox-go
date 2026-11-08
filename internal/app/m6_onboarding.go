package app

// M6 · 引导。The questionnaire runs once, before the shell, and writes the
// kitchen profile the private kitchen and the schedule read.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// onboardingState is the first-run questionnaire. The names stored for
// cookware, restrictions, age, spiciness and saltiness match the Flutter
// project, so a v2 backup imports into the same rows.
type onboardingState struct {
	required bool
	loaded   bool

	step         int // 0..3 through onboardingStepLabels
	stove        int // segmented index: 0..3 are 1..4 stoves
	cookware     map[string]bool
	hometown     string
	age          int // index into onboardingAgeRanges, 0 = 不限
	ageLabel     string
	spicy        int
	salty        int
	restrictions map[string]bool
	allergy      string
	adults       int
	kids         int
	elders       int
}

func (s *onboardingState) init() {
	s.cookware = map[string]bool{}
	s.restrictions = map[string]bool{}
	s.step = 0
	s.stove = 1
	s.ageLabel = onboardingAgeRanges[0].Label
	s.spicy = 1
	s.salty = 1
	s.adults = 2
}

// onboardingCookware is the cookware list the questionnaire and the stored
// JSON share. Names match the Flutter enum names.
var onboardingCookware = []struct{ Name, Label string }{
	{"wok", "炒锅"},
	{"pot", "汤锅"},
	{"pan", "平底锅"},
	{"steamer", "蒸锅"},
	{"nonstickPan", "不粘锅"},
	{"clayPot", "砂锅"},
	{"riceCooker", "电饭煲"},
	{"airFryer", "空气炸锅"},
	{"oven", "烤箱"},
	{"microwave", "微波炉"},
}

// onboardingRestrictions matches DietaryRestriction in the Flutter project.
var onboardingRestrictions = []struct{ Name, Label string }{
	{"vegetarian", "吃素"},
	{"halal", "清真"},
	{"allergy", "有过敏"},
}

// onboardingAgeRanges matches AgeRange in the Flutter project.
var onboardingAgeRanges = []struct{ Name, Label string }{
	{"", "不限"},
	{"under18", "18 岁以下"},
	{"age18to25", "18–25"},
	{"age26to35", "26–35"},
	{"age36to45", "36–45"},
	{"age46to55", "46–55"},
	{"above55", "55 岁以上"},
}

var onboardingSpicy = []struct{ Name, Label string }{
	{"none", "不辣"},
	{"mild", "微辣"},
	{"medium", "中辣"},
	{"hot", "重辣"},
}

var onboardingSalty = []struct{ Name, Label string }{
	{"light", "清淡"},
	{"moderate", "适中"},
	{"heavy", "重口"},
}

// onboardingHometowns is 不限 plus the province-level divisions.
var onboardingHometowns = []string{
	"不限",
	"北京市", "天津市", "河北省", "山西省", "内蒙古自治区",
	"辽宁省", "吉林省", "黑龙江省", "上海市", "江苏省",
	"浙江省", "安徽省", "福建省", "江西省", "山东省",
	"河南省", "湖北省", "湖南省", "广东省", "广西壮族自治区",
	"海南省", "重庆市", "四川省", "贵州省", "云南省",
	"西藏自治区", "陕西省", "甘肃省", "青海省", "宁夏回族自治区",
	"新疆维吾尔自治区", "香港特别行政区", "澳门特别行政区", "台湾省",
}

// onboardingFrom seeds the questionnaire from the stored profile, so a
// restart in the middle of it — or 重做引导 — shows the previous answers.
func onboardingFrom(up domain.UserProfile) onboardingState {
	o := onboardingState{
		required:     !up.Onboarded,
		loaded:       true,
		stove:        up.StoveCount - 1,
		cookware:     map[string]bool{},
		hometown:     up.Hometown,
		allergy:      up.AllergyNote,
		restrictions: map[string]bool{},
		spicy:        1,
		salty:        1,
		ageLabel:     onboardingAgeRanges[0].Label,
		adults:       2,
	}
	if o.stove < 0 || o.stove > 3 {
		o.stove = 1
	}
	if up.Cookware != "" {
		var counts map[string]int
		if json.Unmarshal([]byte(up.Cookware), &counts) == nil {
			for name, n := range counts {
				o.cookware[name] = n > 0
			}
		}
	}
	for _, name := range strings.Split(up.Restrictions, ",") {
		if name = strings.TrimSpace(name); name != "" {
			o.restrictions[name] = true
		}
	}
	if up.HouseholdMix != "" {
		var mix struct{ Adult, Kid, Elder int }
		if json.Unmarshal([]byte(up.HouseholdMix), &mix) == nil {
			o.adults = max(mix.Adult, 0)
			o.kids = max(mix.Kid, 0)
			o.elders = max(mix.Elder, 0)
		}
	}
	for i, ar := range onboardingAgeRanges {
		if ar.Name == up.AgeRange {
			o.age = i
			o.ageLabel = ar.Label
		}
	}
	for i, s := range onboardingSpicy {
		if s.Name == up.Spicy {
			o.spicy = i
		}
	}
	for i, s := range onboardingSalty {
		if s.Name == up.Saltiness {
			o.salty = i
		}
	}
	if up.Hometown == "" {
		o.hometown = "不限"
	}
	return o
}

// toProfile turns the answers into the row the database stores.
func (o *onboardingState) toProfile() domain.UserProfile {
	cookware := make(map[string]int, len(onboardingCookware))
	for _, c := range onboardingCookware {
		if o.cookware[c.Name] {
			cookware[c.Name] = 1
		}
	}
	encodedCookware, _ := json.Marshal(cookware)
	restrictions := make([]string, 0, len(onboardingRestrictions))
	for _, r := range onboardingRestrictions {
		if o.restrictions[r.Name] {
			restrictions = append(restrictions, r.Name)
		}
	}
	mix, _ := json.Marshal(map[string]int{
		"adult": o.adults, "kid": o.kids, "elder": o.elders,
	})
	hometown := strings.TrimSpace(o.hometown)
	if hometown == "不限" {
		hometown = ""
	}
	age := onboardingAgeRanges[clampIndex(o.age, len(onboardingAgeRanges))].Name
	spicy := onboardingSpicy[clampIndex(o.spicy, len(onboardingSpicy))].Name
	salty := onboardingSalty[clampIndex(o.salty, len(onboardingSalty))].Name
	return domain.UserProfile{
		StoveCount:    o.stove + 1,
		Cookware:      string(encodedCookware),
		Hometown:      hometown,
		AgeRange:      age,
		Spicy:         spicy,
		Saltiness:     salty,
		Restrictions:  strings.Join(restrictions, ","),
		AllergyNote:   strings.TrimSpace(o.allergy),
		HouseholdSize: o.adults + o.kids + o.elders,
		HouseholdMix:  string(mix),
		Onboarded:     true,
		Version:       1,
	}
}

// onboardingRequired reports whether the first-run questionnaire is still
// due. The shell shows the questionnaire instead of the sidebar until it is
// answered or skipped.
func (a *App) onboardingRequired() bool {
	o := &a.prefs.onboarding
	return o.loaded && o.required
}

// finishOnboarding saves the answers and enters the shell.
func (a *App) finishOnboarding() {
	o := &a.prefs.onboarding
	profile := o.toProfile()
	o.required = false
	store := a.store
	if store == nil {
		a.notice = "问卷已记录（未连接数据库）"
		a.router.Replace(a.homePath())
		return
	}
	go func() {
		err := store.SaveOnboarding(context.Background(), profile)
		a.post(func() {
			if err != nil {
				a.notice = "问卷保存失败：" + err.Error()
			}
			a.router.Replace(a.homePath())
		})
	}()
}

// skipOnboarding records the default profile — two stoves, wok and pot, no
// restrictions — and marks the questionnaire done.
func (a *App) skipOnboarding() {
	a.prefs.onboarding = onboardingFrom(domain.UserProfile{})
	a.prefs.onboarding.required = false
	a.finishOnboarding()
}

// undoOnboarding shows the questionnaire again from 设置.
func (a *App) undoOnboarding() {
	a.prefs.onboarding.required = true
}

// onboardingStepLabels are the four stops of the first-run questionnaire.
var onboardingStepLabels = []string{"厨房", "家人", "口味", "确认"}

var (
	onboardingSpicyLabels = []string{"不辣", "微辣", "中辣", "重辣"}
	onboardingSaltyLabels = []string{"清淡", "适中", "重口"}
)

// onboardingView is the full-window questionnaire. One stop shows at a
// time, with a progress line and a fixed footer, so a first launch reads
// as a short conversation rather than a long form.
func (a *App) onboardingView(c *ui.Context, p design.Palette) {
	o := &a.prefs.onboarding
	c.Root().LinearGradient(p.Wash)
	ui.Column(c).Fill().Children(func() {
		a.onboardingHeader(c, p)
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(24, 8, 24, 24).Children(func() {
				ui.Column(c).Width(940).Gap(16).Children(func() {
					switch o.step {
					case 0:
						a.onboardingKitchen(c, p)
					case 1:
						a.onboardingHousehold(c, p)
					case 2:
						a.onboardingTaste(c, p)
					default:
						a.onboardingReview(c, p)
					}
				})
			})
		})
		a.onboardingFooter(c, p)
	})
}

// onboardingHeader keeps the escape hatch and the progress line in place
// above the scrolling step.
func (a *App) onboardingHeader(c *ui.Context, p design.Palette) {
	o := &a.prefs.onboarding
	ui.Row(c).FillWidth().Justify(ui.Center).Padding(28, 22, 12, 22).Children(func() {
		ui.Row(c).Width(940).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(6).MinWidth(0).Children(func() {
				ui.Text(c, "开始使用食匣").FontSize(28).FontWeight(700).TextColor(p.Text)
				ui.Text(c, "花一分钟说说你的厨房，掌序会按灶台和锅具排时间，私厨会按口味避忌。").
					FontSize(13).TextColor(p.Muted)
				ui.Row(c).Gap(16).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
					for i, label := range onboardingStepLabels {
						color := p.Faint
						if i == o.step {
							color = p.Accent
						} else if i < o.step {
							color = p.Success
						}
						ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
							ui.Box(c).Size(7, 7).Radius(design.RadiusPill).Background(color)
							ui.Text(c, label).FontSize(design.FontMeta).FontWeight(600).
								TextColor(color)
						})
					}
				})
			})
			if design.Button(c, p, "onboard-skip", "跳过",
				design.ButtonSecondary).Clicked() {
				a.skipOnboarding()
			}
		})
	})
}

func (a *App) onboardingKitchen(c *ui.Context, p design.Palette) {
	o := &a.prefs.onboarding
	a.onboardingSection(c, p, "厨房设备", "同时能开几个灶、有哪些锅", func() {
		design.FieldLabel(c, p, "灶台数量")
		design.Segmented(c, &o.stove, "1 个", "2 个", "3 个", "4 个").Label("灶台数量")
		design.FieldLabel(c, p, "锅具（可多选）")
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			for _, cw := range onboardingCookware {
				name := cw.Name
				chip := design.Chip(c, p, "onboard-cookware-"+name, cw.Label, o.cookware[name])
				if chip.Clicked() {
					o.cookware[name] = !o.cookware[name]
				}
			}
		})
	})
}

func (a *App) onboardingHousehold(c *ui.Context, p design.Palette) {
	o := &a.prefs.onboarding
	a.onboardingSection(c, p, "家里几口人", "按成人 / 小孩 / 老人分开记", func() {
		people := []struct {
			label string
			key   string
			value *int
		}{
			{"成人", "onboard-adults", &o.adults},
			{"小孩", "onboard-kids", &o.kids},
			{"老人", "onboard-elders", &o.elders},
		}
		for _, person := range people {
			person := person
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, person.label).FontSize(12).TextColor(p.Muted).Width(40)
				design.StepperNamed(c, p, person.key, person.label+"人数",
					person.value, 0, 9, "人", 130)
			})
		}
		ui.Text(c, "一共 "+fmtCount(o.adults+o.kids+o.elders)+" 位。配餐与掌序份数会按这个人数来。").
			FontSize(design.FontSmall).TextColor(p.Muted)
	})
}

func (a *App) onboardingTaste(c *ui.Context, p design.Palette) {
	o := &a.prefs.onboarding
	a.onboardingSection(c, p, "口味底子", "按籍贯和口味调整推荐", func() {
		design.FieldLabel(c, p, "籍贯")
		ui.Select(c, &o.hometown, onboardingHometowns).Label("籍贯")
		design.FieldLabel(c, p, "年龄段")
		labels := make([]string, len(onboardingAgeRanges))
		for i, ar := range onboardingAgeRanges {
			labels[i] = ar.Label
		}
		if ui.Select(c, &o.ageLabel, labels).Label("年龄段").Changed() {
			for i, label := range labels {
				if label == o.ageLabel {
					o.age = i
				}
			}
		}
		design.FieldLabel(c, p, "辣度")
		design.Segmented(c, &o.spicy, onboardingSpicyLabels...).Label("辣度")
		design.FieldLabel(c, p, "咸度")
		design.Segmented(c, &o.salty, onboardingSaltyLabels...).Label("咸度")
		design.FieldLabel(c, p, "忌口（可多选）")
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			for _, r := range onboardingRestrictions {
				name := r.Name
				chip := design.Chip(c, p, "onboard-restriction-"+name, r.Label, o.restrictions[name])
				if chip.Clicked() {
					o.restrictions[name] = !o.restrictions[name]
				}
			}
		})
		design.FieldLabel(c, p, "过敏源")
		ui.TextInput(c, &o.allergy).Placeholder("如：花生、虾、牛奶；没有就留空").Label("过敏源")
	})
}

func (a *App) onboardingReview(c *ui.Context, p design.Palette) {
	o := &a.prefs.onboarding
	a.onboardingSection(c, p, "确认一下", "这些答案只存本机，可以随时在设置里重做", func() {
		cookware := []string{}
		for _, cw := range onboardingCookware {
			if o.cookware[cw.Name] {
				cookware = append(cookware, cw.Label)
			}
		}
		cookwareText := "未选"
		if len(cookware) > 0 {
			cookwareText = strings.Join(cookware, "、")
		}
		restrictions := []string{}
		for _, r := range onboardingRestrictions {
			if o.restrictions[r.Name] {
				restrictions = append(restrictions, r.Label)
			}
		}
		restrictionText := "无"
		if len(restrictions) > 0 {
			restrictionText = strings.Join(restrictions, "、")
		}
		hometown := o.hometown
		if hometown == "" {
			hometown = "不限"
		}
		allergy := o.allergy
		if allergy == "" {
			allergy = "无"
		}
		rows := []struct{ label, value string }{
			{"灶台", fmt.Sprintf("%d 个", o.stove+1)},
			{"锅具", cookwareText},
			{"家人", fmt.Sprintf("%s 位（成人 %s / 小孩 %s / 老人 %s）",
				fmtCount(o.adults+o.kids+o.elders),
				fmtCount(o.adults), fmtCount(o.kids), fmtCount(o.elders))},
			{"籍贯", hometown},
			{"年龄段", o.ageLabel},
			{"辣度", onboardingSpicyLabels[clampIndex(o.spicy, len(onboardingSpicyLabels))]},
			{"咸度", onboardingSaltyLabels[clampIndex(o.salty, len(onboardingSaltyLabels))]},
			{"忌口", restrictionText},
			{"过敏源", allergy},
		}
		for _, row := range rows {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, row.label).FontSize(design.FontSmall).TextColor(p.Muted).
					Width(64).Shrink(0)
				ui.Text(c, row.value).FontSize(design.FontBody).TextColor(p.Text).
					Grow(1).Wrap()
			})
		}
		ui.Row(c).FillWidth().Gap(8).Margin(8, 0, 0, 0).Children(func() {
			if design.Button(c, p, "onboard-restart", "重新填写",
				design.ButtonGhost).Clicked() {
				o.step = 0
			}
			ui.Text(c, "跳过会按默认档案记下：2 个灶台、炒锅和汤锅各一把。").
				FontSize(design.FontMeta).TextColor(p.Faint)
		})
	})
}

// onboardingFooter is the fixed step bar: back, next, and finish on the
// last stop. It sits below the scroll, so it never covers a field.
func (a *App) onboardingFooter(c *ui.Context, p design.Palette) {
	o := &a.prefs.onboarding
	bar := ui.Row(c).FillWidth().Padding(14, 22, 14, 22).Background(p.Surface).
		Shadow(0, -8, 22, -14, p.Shadow)
	bar.Children(func() {
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(1).Background(p.Border)
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Row(c).Width(940).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				ui.Text(c, fmt.Sprintf("第 %d / %d 步 · %s",
					o.step+1, len(onboardingStepLabels), onboardingStepLabels[o.step])).
					FontSize(design.FontMeta).TextColor(p.Muted)
				ui.Row(c).Gap(8).Children(func() {
					if o.step > 0 {
						if design.Button(c, p, "onboard-prev", "上一步",
							design.ButtonSecondary).Clicked() {
							o.step--
						}
					}
					if o.step < len(onboardingStepLabels)-1 {
						if design.Button(c, p, "onboard-next", "下一步",
							design.ButtonPrimary).Clicked() {
							o.step++
						}
					} else if design.Button(c, p, "onboard-done", "完成并进入",
						design.ButtonPrimary).Clicked() {
						a.finishOnboarding()
					}
				})
			})
		})
	})
}

// onboardingSection is one card of the questionnaire.
func (a *App) onboardingSection(c *ui.Context, p design.Palette, title, sub string, build func()) {
	box := design.CardPad(c, p)
	box.Children(func() {
		ui.Text(c, title).FontSize(15).FontWeight(700).TextColor(p.Text)
		ui.Text(c, sub).FontSize(12).TextColor(p.Muted)
		build()
	})
}

// clampIndex keeps an index inside [0, n).
func clampIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// fmtCount renders a household count as a whole number.
func fmtCount(v int) string {
	return fmt.Sprintf("%d", v)
}
