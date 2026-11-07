package app

// One-time import of a Flutter/Drift FoodBox database. The source file is
// opened read-only; the current data is backed up first and replaced in one
// transaction, the same way a JSON restore works.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/data/backup"
	"github.com/gij072858-cmd/foodbox-go/internal/data/legacy"
	"github.com/gij072858-cmd/foodbox-go/internal/design"
)

// legacyImportState is the import page's state.
type legacyImportState struct {
	path    string
	summary legacy.Summary
	busy    bool
}

func (s *legacyImportState) init() {}

func (a *App) legacyImportPage(c *ui.Context, p design.Palette) {
	a.pageBody(c, func() {
		a.subHeader(c, p, "导入旧版数据库", "从 Flutter 版食匣的 .sqlite 文件迁移")
		ui.Text(c, "读取旧库的十张表：食材、菜式、步骤、掌序、记录、档案、日记。导入前会自动备份当前数据；旧库文件只读，不会被修改。").FontSize(12).TextColor(p.Muted).LineHeight(1.5)

		card := ui.Column(c).FillWidth().Gap(design.Sp(c, 2))
		card.Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.PrimaryButton(c, "选择旧版数据库…").Clicked() {
					a.pickLegacyFile()
				}
				if a.legacy.busy {
					loading.Arc(c, loading.Options{Size: 18, Color: p.Accent})
					ui.Text(c, "正在读取…").FontSize(12).TextColor(p.Muted)
				}
			})

			if a.legacy.path == "" {
				ui.Text(c, "常见位置：Android 的 foodbox.sqlite，或桌面版用户数据目录里的 foodbox.db。").FontSize(11).TextColor(p.Faint)
				return
			}
			ui.Text(c, a.legacy.path).FontSize(11).TextColor(p.Faint).SingleLine()
			if a.legacy.busy {
				return
			}
			if a.legacy.summary.Empty() {
				ui.Text(c, "这个文件里没有找到食匣的菜式或食材表，换一个文件试试。").FontSize(12).TextColor(p.Warning)
				return
			}
			ui.Text(c, a.legacy.summary.Counts()).FontSize(13).FontWeight(600).TextColor(p.Text)
			if len(a.legacy.summary.Tables) > 0 {
				ui.Text(c, "来源表："+strings.Join(a.legacy.summary.Tables, "、")).FontSize(11).TextColor(p.Muted).LineHeight(1.5)
			}
			if ui.Button(c, "导入这些数据").Clicked() {
				a.importLegacy()
			}
		})

	})
}

// pickLegacyFile asks for the old database and inspects it off the view.
func (a *App) pickLegacyFile() {
	if a.win == nil {
		a.notice = "窗口尚未就绪"
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.win,
		Title:  "选择食匣旧版数据库",
		Filters: []mygo.FileFilter{
			{Name: "SQLite 数据库", Extensions: []string{"sqlite", "db", "sqlite3"}},
			{Name: "所有文件", Extensions: []string{"*"}},
		},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	path := paths[0]
	a.legacy.path = path
	a.legacy.summary = legacy.Summary{}
	a.legacy.busy = true
	a.notice = ""
	go func() {
		sum, err := legacy.Inspect(context.Background(), path)
		a.post(func() {
			a.legacy.busy = false
			if err != nil {
				a.notice = "读取旧库失败：" + err.Error()
				return
			}
			a.legacy.summary = sum
		})
	}()
}

// importLegacy backs up the current data, then replaces it from the legacy
// file in one transaction.
func (a *App) importLegacy() {
	store := a.store
	if store == nil || a.win == nil {
		a.notice = "数据库尚未就绪"
		return
	}
	sum := a.legacy.summary
	choice, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent: a.win,
		Type:   mygo.MessageWarning,
		Title:  "导入会覆盖当前数据",
		Message: fmt.Sprintf("旧库含 %d 道菜、%d 样食材、%d 条记录。",
			sum.Recipes, sum.Items, sum.Records),
		Detail:             "导入前会自动备份当前数据；图片和 AI 密钥不在备份里。",
		Buttons:            []string{"取消", "导入"},
		DestructiveButtons: []int{1},
	})
	if err != nil || choice.Button != 1 {
		return
	}
	path := a.legacy.path
	a.legacy.busy = true
	go func() {
		autoPath := ""
		if dir := a.backupDir(); dir != "" {
			if current, err := backup.Export(context.Background(), store, appVersion()); err == nil {
				autoPath = filepath.Join(dir, "auto-"+time.Now().Format("20060102-150405")+".json")
				if err := backup.WriteFile(autoPath, current); err == nil {
					pruneBackups(dir, 5)
				} else {
					autoPath = ""
				}
			}
		}
		imported, err := legacy.Import(context.Background(), path, store)
		if err == nil {
			err = a.reloadAfterRestore(store)
		}
		a.post(func() {
			a.legacy.busy = false
			if err != nil {
				a.notice = "导入失败，原数据未改动：" + err.Error()
				return
			}
			a.legacy.summary = imported
			a.notice = fmt.Sprintf("已导入 %d 道菜、%d 个步骤、%d 样食材", imported.Recipes, imported.Steps, imported.Items)
			if autoPath != "" {
				a.notice += "；原数据已备份到 " + filepath.Base(autoPath)
			}
			a.router.Replace("/pantry")
		})
	}()
}
