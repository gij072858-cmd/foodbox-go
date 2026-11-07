package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/egoist/mygo"

	"github.com/gij072858-cmd/foodbox-go/internal/data/backup"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
)

// backupDir is <user data>/backups. It holds pre-import automatic backups;
// media and AI keys are not part of a backup.
func (a *App) backupDir() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "backups")
}

// exportBackup writes a format-3 snapshot chosen with the system save
// dialog. Empty paths (cancel) do nothing.
func (a *App) exportBackup() {
	if a.store == nil || a.win == nil {
		a.notice = "数据库尚未就绪"
		return
	}
	dir := a.backupDir()
	name := "食匣备份-" + time.Now().Format("20060102-150405") + ".json"
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       "导出食匣备份",
		DefaultPath: filepath.Join(dir, name),
		Filters:     []mygo.FileFilter{{Name: "食匣备份", Extensions: []string{"json"}}},
	})
	if err != nil || path == "" {
		return
	}
	store := a.store
	go func() {
		snap, err := backup.Export(context.Background(), store, appVersion())
		if err == nil {
			err = backup.WriteFile(path, snap)
		}
		a.post(func() {
			if err != nil {
				a.notice = "导出失败：" + err.Error()
				return
			}
			a.notice = fmt.Sprintf("已导出 %d 道菜、%d 样食材到 %s",
				snap.Counts["recipes"], snap.Counts["items"], filepath.Base(path))
		})
	}()
}

// importBackup restores after a confirmation and an automatic backup of the
// current data. The import itself is one transaction.
func (a *App) importBackup() {
	if a.store == nil || a.win == nil {
		a.notice = "数据库尚未就绪"
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:  a.win,
		Title:   "导入食匣备份",
		Filters: []mygo.FileFilter{{Name: "食匣备份", Extensions: []string{"json"}}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	path := paths[0]
	snap, err := backup.ReadFile(path)
	if err != nil {
		a.notice = "无法导入：" + err.Error()
		return
	}
	choice, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent: a.win,
		Type:   mygo.MessageWarning,
		Title:  "导入会覆盖当前数据",
		Message: fmt.Sprintf("文件含 %d 道菜、%d 样食材、%d 条记录。",
			snap.Counts["recipes"], snap.Counts["items"], snap.Counts["records"]),
		Detail:             "导入前会自动备份当前数据。图片和 AI 密钥不在备份里。",
		Buttons:            []string{"取消", "导入"},
		DestructiveButtons: []int{1},
	})
	if err != nil || choice.Button != 1 {
		return
	}
	store := a.store
	go func() {
		autoPath := ""
		if dir := a.backupDir(); dir != "" {
			if current, err := backup.Export(context.Background(), store, appVersion()); err == nil {
				autoPath = filepath.Join(dir, "auto-"+time.Now().Format("20060102-150405")+".json")
				if err := backup.WriteFile(autoPath, current); err == nil {
					pruneBackups(dir, 5)
				}
			}
		}
		err := backup.Import(context.Background(), store, snap)
		if err == nil {
			err = a.reloadAfterRestore(store)
		}
		a.post(func() {
			if err != nil {
				a.notice = "导入失败，原数据未改动：" + err.Error()
				return
			}
			if autoPath != "" {
				a.notice = "导入完成；原数据已备份到 " + filepath.Base(autoPath)
			} else {
				a.notice = "导入完成"
			}
		})
	}()
}

// reloadAfterRestore reloads every snapshot after the restore transaction.
func (a *App) reloadAfterRestore(store repository.Repository) error {
	ctx := context.Background()
	recipes, err := store.ListAll(ctx)
	if err != nil {
		return err
	}
	steps, err := store.AllSteps(ctx)
	if err != nil {
		return err
	}
	items, err := store.ListItems(ctx)
	if err != nil {
		return err
	}
	schedule, err := store.ListSchedule(ctx)
	if err != nil {
		return err
	}
	records, err := store.ListRecords(ctx, 200)
	if err != nil {
		return err
	}
	recordCount, err := store.RecordCount(ctx)
	if err != nil {
		return err
	}
	mealDays, err := store.MealDayCount(ctx)
	if err != nil {
		return err
	}
	if err := a.loadFeatureState(ctx, store); err != nil {
		return err
	}
	byID := make(map[int64]domain.Recipe, len(recipes))
	for _, r := range recipes {
		byID[r.ID] = r
	}
	a.post(func() {
		a.recipes = recipes
		a.recipeByID = byID
		a.steps = steps
		a.setItems(items)
		a.schedule = schedule
		a.setRecords(records, recordCount, mealDays)
	})
	return nil
}

// pruneBackups keeps the newest n auto-*.json files.
func pruneBackups(dir string, n int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 5 && e.Name()[:5] == "auto-" {
			names = append(names, e.Name())
		}
	}
	if len(names) <= n {
		return
	}
	sort.Strings(names)
	for _, name := range names[:len(names)-n] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
