package app

// M6 · 帮助。Static, offline help pages: grouped notes on every section and
// the shortcuts the shell and the run page accept.

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
)

// helpState is which help sections are open.
type helpState struct {
	open []bool
}

func (s *helpState) init() {
	s.open = make([]bool, len(helpSections))
	if len(s.open) > 0 {
		s.open[0] = true // the pantry notes are the ones a new user reads first
	}
}

// helpSection is one collapsible group on the help page.
type helpSection struct {
	Title string
	Sub   string
	Lines []string
}

var helpSections = []helpSection{
	{
		Title: "仓库",
		Sub:   "记录手头有什么，先吃快过期的",
		Lines: []string{
			"「仓库」列出全部在库食材；标「先吃我」的是临期或已经过期的条目。",
			"从「系统目录」添加条目会带上分类、保存方式与保质期；自建条目可以只填名称。",
			"数量可以记精确值，也可以只记「充足 / 有一些 / 快没了」三档。",
			"「盘点核销」逐项确认：还在就点「还有」，用完点「用完」；用完的条目保留为灰卡，随时能恢复。",
			"「购物清单」汇总已用完的条目，以及已加入掌序、但仓库里没有的必需食材。",
		},
	},
	{
		Title: "菜式",
		Sub:   "944 道菜，按主料、菜系、做法找",
		Lines: []string{
			"菜式页按库存把每道菜标成「现在能做 / 差一点 / 差太多」；差一点的会列出缺哪几样。",
			"左侧搜索同时过滤菜名与食材名；时令与排序只改列表，不改数据。",
			"详情页能看到食材、步骤、完成标志，以及同一版本组的其他做法。",
			"「加入掌序」把菜放进待做队列；「加入购物清单」把缺的必需食材记下来。",
		},
	},
	{
		Title: "掌序",
		Sub:   "把多道菜的步骤排成一条时间线",
		Lines: []string{
			"掌序是待做队列：每道菜显示份数与预计时长，可以随时移出。",
			"「备料清单」按生鲜、底料、调料分组，标出在库数量与缺口。",
			"「排程时间线」按灶台与锅具把步骤并行排开，顶部显示总时长与并行节省。",
			"「开始掌序」进入歌词流：当前一步居中，左右键翻步，可以暂停与挂起计时。",
			"「结算」写入做菜记录、扣库存、写当天的开饭日记，并提高等级。",
		},
	},
	{
		Title: "私厨",
		Sub:   "按身体档案与口味配餐",
		Lines: []string{
			"先填「身体档案」：身高体重、活动量、目标与营养目标；缺数据时不会编造热量。",
			"「同桌成员」记录一起吃饭的人与忌口，配餐份数按人数来。",
			"两菜一汤与一碗端都由本地策略生成，约束（清真、吃素、痛风等）是硬过滤。",
			"「AI 与配额」里的密钥只存本机凭据管理器，不进数据库、不进备份；没配密钥或超配额时回退本地策略。",
		},
	},
	{
		Title: "数据与备份",
		Sub:   "数据只在本机，备份不含密钥",
		Lines: []string{
			"所有数据存在本机的 SQLite 数据库里，没有账号也没有云端同步。",
			"「导出备份」生成一个 JSON 文件，含十张表；照片只存路径，不打包图片。",
			"「导入备份」会先自动备份当前数据并保留最近 5 份，再整体覆盖；可以导入旧版 Flutter 备份。",
			"AI 密钥不在备份里，换机后需要在设置里重新填写。",
			"「做过的菜」页可以一键清空记录，收藏与等级保留。",
		},
	},
	{
		Title: "快捷键",
		Sub:   "键盘能到的地方",
		Lines: []string{
			"Tab / Shift+Tab：在可点元素之间移动焦点；Enter 或空格触发。",
			"Esc：关闭当前弹层或确认框。",
			"掌序执行页：← / → 切换上一步与下一步；空格暂停或继续。",
			"弹窗里最后一个按钮是默认按钮，Enter 直接触发，Esc 等于取消。",
			"Ctrl/⌘ + F：跳到左侧搜索框；输入后菜式列表即时过滤。",
			"Ctrl/⌘ + 1…7：按侧栏顺序在仓库、菜式、私厨、掌序、食历、营养、我的之间切换。",
		},
	},
}

// helpLinesFor drops the pantry notes when the pantry is closed, so the help
// page never teaches a door that is not there.
func helpLinesFor(section helpSection, pantryOff bool) []string {
	if !pantryOff {
		return section.Lines
	}
	drop := func(line string) bool {
		return strings.Contains(line, "库存") || strings.Contains(line, "在库") ||
			strings.Contains(line, "购物清单") || strings.Contains(line, "扣库存")
	}
	out := make([]string, 0, len(section.Lines))
	for _, line := range section.Lines {
		if drop(line) {
			continue
		}
		if section.Title == "快捷键" && strings.Contains(line, "1…7") {
			line = "Ctrl/⌘ + 1…6：按侧栏顺序在菜式、私厨、掌序、食历、营养、我的之间切换。"
		}
		out = append(out, line)
	}
	return out
}

// helpPage draws the grouped help.
func (a *App) helpPage(c *ui.Context, p design.Palette) {
	h := &a.prefs.help
	a.pageBody(c, func() {
		ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
			ui.Text(c, "帮助").FontSize(22).FontWeight(700).TextColor(p.Text)
			ui.Text(c, "本地使用说明，按分组展开；内容只讲怎么用，不解释实现。").
				FontSize(12).TextColor(p.Muted)
		})
		ui.Accordion(c, func() {
			for i, section := range helpSections {
				i, section := i, section
				if i >= len(h.open) {
					continue
				}
				if a.pantryOff && section.Title == "仓库" {
					continue
				}
				lines := helpLinesFor(section, a.pantryOff)
				ui.AccordionItem(c, section.Title, &h.open[i], func() {
					if section.Sub != "" {
						ui.Text(c, section.Sub).FontSize(12).TextColor(p.Muted)
					}
					for _, line := range lines {
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
							ui.Text(c, "·").FontSize(13).TextColor(p.Faint)
							ui.Text(c, line).FontSize(13).TextColor(p.Text).LineHeight(1.6).Grow(1)
						})
					}
				})
			}
		})
	})
}
