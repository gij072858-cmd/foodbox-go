package app

import (
	"fmt"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/videoproxy"
)

// appVersion is the version MyGo packaged from mygo.json, with a fallback
// for `go run`, where the binary has no package metadata.
func appVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return "0.1.0"
}

// settingsChevron is the mark every navigating settings row wears. It is
// drawn by the page rather than by design, because the icon set lives in
// this package.
func settingsChevron(c *ui.Context, p design.Palette) {
	ui.Icon(c, &icons.Chevron).Size(14, 14).TextColor(p.Faint)
}

// settingsRule is the hairline between two rows of one settings group.
func settingsRule(c *ui.Context) {
	ui.Divider(c).Margin(0, 14, 0, 14)
}

// settingsPage is 设置: appearance, the kitchen entries, AI, backup and about.
// Every group is open and always shown; the hierarchy is carried by a small
// group label over full-weight row titles, not by a disclosure arrow.
func (a *App) settingsPage(c *ui.Context, p design.Palette) {
	a.pageBody(c, func() {
		design.SettingsGroup(c, p, "外观与动效", func() {
			design.SettingsRow(c, p, "外观", "跟随系统，或固定浅色 / 深色", func() {
				if design.Segmented(c, &a.themeIndex, themeLabels...).
					Label("外观").Changed() {
					a.theme = themeName(a.themeIndex)
					a.applyNativeTheme()
					a.savePrefs()
				}
			})
			settingsRule(c)
			design.SettingsRow(c, p, "主题", "浅色外观下的窗底、卡片与强调色；深色固定为素色", func() {
				// The picker names the wash rather than painting it: ten
				// previews of ten near-identical gradients asked the user to
				// spot the difference, and dark no longer follows the theme
				// at all, so half of them were always a lie. Its trigger is
				// a menu, not a banner, so it keeps a menu's width.
				themeLabel := design.ThemeLabel(a.themeName)
				if ui.Select(c.Key("theme-select"), &themeLabel, design.ThemeOptions()).
					Label("主题").Width(240).AlignSelf(ui.Start).Changed() {
					if name := design.ThemeNameForLabel(themeLabel); name != "" && name != a.themeName {
						a.themeName = name
						a.savePrefs()
					}
				}
			})
			settingsRule(c)
			design.SettingsRow(c, p, "动效强度",
				"跟随系统「减少动态效果」；关闭后页面切换与悬浮动画都会停用", func() {
					if design.Segmented(c, &a.motionIndex, motionLabels...).
						Label("动效强度").Changed() {
						a.motion = motionName(a.motionIndex)
						a.savePrefs()
					}
				})
		})

		design.SettingsGroup(c, p, "厨房档案", func() {
			if row := design.SettingsLinkRow(c, p, "settings-body", "身体档案",
				"身高体重、活动量、营养目标",
				func() { settingsChevron(c, p) }); row.Clicked() {
				a.router.Push("/sichu/body")
			}
			settingsRule(c)
			if row := design.SettingsLinkRow(c, p, "settings-members", "同桌成员",
				"一起吃饭的人、忌口与目标",
				func() { settingsChevron(c, p) }); row.Clicked() {
				a.router.Push("/sichu/members")
			}
			settingsRule(c)
			design.SettingsRow(c, p, "仓库页",
				"关闭后隐藏仓库、购物与库存相关入口；掌序结算不再扣库存，私厨从全部菜式配餐", func() {
					on := a.pantryOff
					if ui.Switch(c, &on).Label("关闭仓库页").Changed() {
						a.setPantryOff(on)
					}
				})
			settingsRule(c)
			design.SettingsRow(c, p, "重做首次引导",
				"会重新打开问卷，答案可以改也可以跳过", func() {
					ui.Row(c).Children(func() {
						if ui.Button(c, "重新打开").Clicked() {
							a.undoOnboarding()
						}
					})
				})
		})

		design.SettingsGroup(c, p, "AI 与配额", func() {
			status := "未配置 · 密钥仅存本机凭据管理器，不进备份"
			if a.ai.configured {
				status = fmt.Sprintf("已配置 · 今日剩余 %d 次", a.ai.remaining)
			}
			if row := design.SettingsLinkRow(c, p, "settings-ai", "接口密钥与模型",
				status, func() { settingsChevron(c, p) }); row.Clicked() {
				a.router.Push("/settings/ai")
			}
		})

		design.SettingsGroup(c, p, "数据与备份", func() {
			design.SettingsRow(c, p, "备份与恢复",
				"备份包含十张表：食材、菜式、步骤、掌序、记录、档案、日记；不含 AI 密钥与照片。"+
					"导入前会自动备份当前数据并保留 5 份。", func() {
					ui.Row(c).Gap(10).Children(func() {
						if ui.PrimaryButton(c, "导出备份").Clicked() {
							a.exportBackup()
						}
						if ui.Button(c, "导入备份…").Clicked() {
							a.importBackup()
						}
					})
				})
			settingsRule(c)
			if row := design.SettingsLinkRow(c, p, "settings-legacy", "导入旧版数据库…",
				"从 Flutter 版食匣的 .sqlite 一次性迁移",
				func() { settingsChevron(c, p) }); row.Clicked() {
				a.router.Push("/settings/legacy")
			}
		})

		design.SettingsGroup(c, p, "帮助", func() {
			if row := design.SettingsLinkRow(c, p, "settings-help", "帮助",
				"仓库、菜式、掌序、私厨、备份与快捷键",
				func() { settingsChevron(c, p) }); row.Clicked() {
				a.router.Push("/help")
			}
		})

		design.SettingsGroup(c, p, "关于", func() {
			ui.Column(c).FillWidth().Gap(4).Padding(12, 14).Children(func() {
				ui.Text(c, "食匣 · FoodBox-Go").FontSize(14).FontWeight(600).TextColor(p.Text)
				ui.Text(c, fmt.Sprintf("版本 %s · MyGo v%s · %s",
					appVersion(), mygo.Version, runtime.Version())).
					FontSize(12).TextColor(p.Muted)
				ui.Text(c, fmt.Sprintf("数据源：原 FoodBox 项目（%d 道菜式、%d 条系统目录）",
					len(a.recipes), len(a.catalog))).FontSize(12).TextColor(p.Muted)
				local := fmt.Sprintf("本机：%d 条开饭记录 · %d 样在库 · %d 道待做",
					a.recordCount, len(a.items), len(a.schedule))
				if a.pantryOff {
					local = fmt.Sprintf("本机：%d 条开饭记录 · %d 道待做",
						a.recordCount, len(a.schedule))
				}
				ui.Text(c, local).FontSize(12).TextColor(p.Muted)
				ui.Text(c, "离线单机，没有账号与云端同步。").FontSize(11).TextColor(p.Faint)
			})
		})

		design.SettingsGroup(c, p, "视频播放", func() {
			// Read the bootstrap probe; finding a player scans the PATH and
			// the disk, which must never run under a frame.
			probe := a.videoProbe
			status := "正在检查播放器…"
			if probe.foundOK {
				status = "没找到播放器：「看视频」会在应用内网页里打开"
				if probe.found.Kind != videoproxy.PlayerNone {
					status = probe.found.Kind.String() + " · " + probe.found.Path
					if probe.found.Portable {
						status += "（食匣自己装的便携版）"
					}
				}
			}
			design.SettingsRow(c, p, "外部播放器", status, nil)
			ui.Column(c).FillWidth().Gap(6).Padding(12, 14).Children(func() {
				ui.Text(c, "哔哩哔哩的页面会先解析出直链再交给播放器，点开就是高画质播放窗，"+
					"不用登录、不用全屏、不会被降清晰度。直链几分钟后失效，所以每次点都重新取。").
					FontSize(design.FontSmall).TextColor(p.Muted).LineHeight(1.5)
				ui.Text(c, "小红书没有可解析的接口，仍然只能在应用内网页里看。"+
					"分轨的 DASH 只有 mpv 能同时播画面与声音；只装了 VLC 时，分轨会退回应用内网页。").
					FontSize(design.FontSmall).TextColor(p.Muted).LineHeight(1.5)
			})
			settingsRule(c)
			have := a.videoProbe.cookie
			cookieStatus := "未填写：只能看游客可见的清晰度"
			if have {
				cookieStatus = "已保存（本机凭据管理器）"
			}
			design.SettingsRow(c, p, "哔哩哔哩 SESSDATA", cookieStatus, func() {
				ui.TextInput(c, &a.videoCookie).Placeholder("登录后从浏览器复制").Width(300).
					AlignSelf(ui.Start)
				ui.Row(c).Gap(10).Children(func() {
					if design.Button(c, p, "video-cookie-save", "保存",
						design.ButtonPrimary).Clicked() {
						if err := a.saveBiliCookie(a.videoCookie); err != nil {
							a.notice = "密钥没能保存：" + err.Error()
						} else {
							a.videoCookie = ""
							a.videoProbe.cookie, a.videoProbe.cookieOK = true, true
							a.notice = "已保存到本机凭据管理器"
						}
					}
					if have {
						if design.Button(c, p, "video-cookie-clear", "清除",
							design.ButtonDanger).Clicked() {
							a.clearBiliCookie()
							a.videoProbe.cookie, a.videoProbe.cookieOK = false, true
							a.notice = "已清除登录 Cookie"
						}
					}
				})
				ui.Text(c, "Cookie 只进本机凭据管理器，不进数据库、不进备份。").
					FontSize(design.FontMeta).TextColor(p.Faint)
			})
		})
	})
}
