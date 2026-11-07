package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/data/seed"
	"github.com/gij072858-cmd/foodbox-go/internal/design"
)

func (a *App) startupView(c *ui.Context, p design.Palette) {
	ui.Column(c).Fill().Center().Gap(12).Padding(40).Children(func() {
		logo := ui.Icon(c, &icons.Logo).Size(64, 64).TextColor(p.Ink)
		if a.level.Moves() {
			// One short arrival, as Cloudy's splash: the mark settles
			// rather than appearing between two database writes.
			k := logo.Keyframes("intro", []ui.Keyframe{
				{Value: 0},
				{At: 600 * time.Millisecond, Value: 1, Ease: ui.EaseOut},
			}, ui.KeyframeOptions{Run: 1})
			s := 0.88 + 0.12*k
			logo.Scale(s, s).Opacity(k)
		}
		ui.Text(c, "食匣").FontSize(design.FontDisplay).FontWeight(700).TextColor(p.Text)
		ui.Text(c, "FOODBOX · 原生桌面版").FontSize(11).FontWeight(600).LetterSpacing(1.6).TextColor(p.Faint)
		ui.Box(c).Height(18)

		if a.phase == PhaseError {
			ui.Text(c, "启动失败").FontSize(15).FontWeight(600).TextColor(p.Danger)
			ui.Text(c, a.bootErr).FontSize(12).TextColor(p.Muted).Width(520).TextAlign(ui.Center).LineHeight(1.5)
			if ui.PrimaryButton(c, "重试").Width(140).Clicked() {
				a.retry()
			}
			return
		}

		fraction := 0.0
		if a.progressTotal > 0 {
			fraction = float64(a.progressDone) / float64(a.progressTotal)
		}
		ui.Progress(c, fraction).Width(360)
		ui.Text(c, stageText(a.stage, a.progressDone, a.progressTotal)).FontSize(12).TextColor(p.Muted)
		ui.Text(c, "944 道菜式 · 本地离线数据").FontSize(11).TextColor(p.Faint)
	})
}

func stageText(stage seed.Stage, done, total int) string {
	switch stage {
	case seed.StageRead:
		return "读取种子数据…"
	case seed.StageParse:
		return "解析菜式与步骤…"
	case seed.StageWrite:
		if total > 0 {
			return fmt.Sprintf("导入菜式 %d / %d…", done, total)
		}
		return "写入数据库…"
	case seed.StageDone:
		return "准备就绪"
	default:
		return "打开数据库…"
	}
}
