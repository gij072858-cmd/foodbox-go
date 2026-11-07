package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
)

func (a *App) probePage(c *ui.Context, p design.Palette) {
	a.pageBody(c, func() {
		ui.Text(c, "下面四个交互是 M0 的手感验证；参数与 Cloudy 的三档动效一致。").FontSize(13).TextColor(p.Muted).Margin(0, 0, 20, 0)

		a.section(c, p, "1 · 卡片悬浮与按压", func() {
			ui.Row(c).FillWidth().Gap(12).Children(func() {
				for i := 0; i < 3; i++ {
					i := i
					a.card(c, p, fmt.Sprintf("probe-card-%d", i), fmt.Sprintf("卡片 %d", i+1), "悬浮上浮，按下回弹", i == 1, func() {
						c.Toast(fmt.Sprintf("卡片 %d 被点击", i+1))
					})
				}
			})
		})

		a.section(c, p, "2 · 页面过渡", func() {
			ui.Row(c).Gap(10).Children(func() {
				if ui.PrimaryButton(c, "打开菜式详情").Clicked() {
					a.router.Push("/recipes/1")
				}
				if ui.Button(c, "再来一次").Clicked() {
					a.router.Replace("/pantry")
					a.router.Push("/probe")
				}
			})
		})

		a.section(c, p, "3 · 弹层", func() {
			if ui.PrimaryButton(c, "打开底层弹层").Width(200).Clicked() {
				a.sheetOpen = true
			}
		})

		a.section(c, p, "4 · 歌词流", func() {
			ui.Column(c).FillWidth().Height(300).Radius(16).Background(p.Surface).
				Border(1, p.Border).Children(func() {
				a.lyricFlow(c, p, 300, demoLyric, lyricOptions{Anchor: 0.5, Lit: true})
			})
		})
	})
}

func (a *App) sheet(c *ui.Context, p design.Palette) {
	design.ModalDialog(c, p, &a.sheetOpen, 420, a.level, func() {
		ui.Text(c, "弹层探针").FontSize(18).FontWeight(700).TextColor(p.Text)
		ui.Text(c, "这是 M0 的弹层探针。掌序的结算在掌序页，这里只验证弹层手感。").FontSize(13).TextColor(p.Muted)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.sheetOpen = false
			}
			if ui.PrimaryButton(c, "知道了").Clicked() {
				a.sheetOpen = false
			}
		})
	})
}
