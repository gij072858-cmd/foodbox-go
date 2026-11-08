package app

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
)

// route dispatches the current path to its page. syncPage reads only the
// first path segment, so a sub-page keeps its section's sidebar entry lit.
// More specific patterns must come before the parameterised ones.
func (a *App) route(c *ui.Context, r *ui.Route, p design.Palette) {
	a.buildingPath = r.Path()
	if a.pantryOff && strings.HasPrefix(r.Path(), "/pantry") {
		// The pantry's doors are closed: no route under it renders, and the
		// cook lands on the dishes instead.
		a.router.Replace("/recipes")
		r.Title("菜式")
		a.recipesPage(c, p)
		return
	}
	switch {
	// Pantry (M2).
	case r.Match("/pantry"):
		r.Title("仓库")
		a.pantryPage(c, p)
	case r.Match("/pantry/catalog"):
		r.Title("系统目录")
		a.pantryCatalogPage(c, p)
	case r.Match("/pantry/starter"):
		r.Title("新手包")
		a.starterPackPage(c, p)
	case r.Match("/pantry/shopping"):
		r.Title("购物清单")
		a.shoppingPage(c, p)
	case r.Match("/pantry/reconcile"):
		r.Title("盘点核销")
		a.reconcilePage(c, p)

	// Recipes (M3).
	case r.Match("/recipes"):
		r.Title("菜式")
		a.recipesPage(c, p)
	case r.Match("/recipes/new"):
		r.Title("新建菜式")
		a.recipeEditorPage(c, p, 0)
	case r.Match("/recipes/{id}/edit"):
		r.Title("编辑菜式")
		id, _ := strconv.ParseInt(r.Param("id"), 10, 64)
		a.recipeEditorPage(c, p, id)
	case r.Match("/recipes/{id}"):
		r.Title("菜式详情")
		a.recipeDetail(c, r, p)

	// Private kitchen (M5).
	case r.Match("/sichu"):
		r.Title("私厨")
		a.sichuPage(c, p)
	case r.Match("/sichu/result"):
		r.Title("私厨配餐")
		a.sichuResultPage(c, p)
	case r.Match("/sichu/body"):
		r.Title("身体档案")
		a.sichuBodyPage(c, p)
	case r.Match("/sichu/members"):
		r.Title("同桌成员")
		a.sichuMembersPage(c, p)

	// Schedule (M4).
	case r.Match("/schedule"):
		r.Title("掌序")
		a.schedulePage(c, p)
	case r.Match("/schedule/prep"):
		r.Title("备料清单")
		a.schedulePrepPage(c, p)
	case r.Match("/schedule/timeline"):
		r.Title("排程时间线")
		a.scheduleTimelinePage(c, p)
	case r.Match("/schedule/run"):
		r.Title("掌序执行")
		a.scheduleRunPage(c, p)

	// Feed (M6).
	case r.Match("/feed"):
		r.Title("食历")
		a.feedPage(c, p)
	case r.Match("/feed/review"):
		r.Title("年度回顾")
		a.feedReviewPage(c, p)
	case r.Match("/feed/{date}"):
		r.Title("开饭日记")
		a.feedDayPage(c, r, p)

	// Profile (M6).
	case r.Match("/profile"):
		r.Title("我的")
		a.profilePage(c, p)
	case r.Match("/profile/edit"):
		r.Title("编辑资料")
		a.profileEditPage(c, p)
	case r.Match("/profile/cooked"):
		r.Title("做过的菜")
		a.profileCookedPage(c, p)
	case r.Match("/profile/shelf"):
		r.Title("我的收藏")
		a.profileShelfPage(c, p)

	// Settings, help and onboarding (M6).
	case r.Match("/settings/ai"):
		r.Title("AI 与配额")
		a.aiSettingsPage(c, p)
	case r.Match("/settings"):
		r.Title("设置")
		a.settingsPage(c, p)
	case r.Match("/help"):
		r.Title("帮助")
		a.helpPage(c, p)
	case r.Match("/onboarding"):
		r.Title("开始使用")
		a.onboardingView(c, p)

	// Nutrition dashboard (M5).
	case r.Match("/nutrition"):
		r.Title("营养驾驶舱")
		a.nutritionPage(c, p)

	// Motion probe (M0).
	case r.Match("/probe"):
		r.Title("动效探针")
		a.probePage(c, p)

	default:
		r.Title("未找到")
		a.pageBody(c, func() {
			ui.Text(c, "页面不存在").FontSize(24).FontWeight(700).TextColor(p.Text)
			label, target := "回到仓库", "/pantry"
			if a.pantryOff {
				label, target = "回到菜式", "/recipes"
			}
			if ui.Button(c, label).Clicked() {
				a.router.Replace(target)
			}
		})
	}
}
