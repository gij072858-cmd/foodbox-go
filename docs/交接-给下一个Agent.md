# 交接：M0–M7 已完成后的接手说明

## 现在的状态

**全功能复刻已完成**：M0–M7 全部落地，`main` 分支可构建、可打包、测试全绿。
最后一次验证：2026-11-05。

- 进度与证据：`docs/进度.md`
- 逐项审计：`docs/验收-M7-全量审计.md`
- 并行施工合同（五条线怎么拆的）：`docs/施工-M3-M7-任务书.md`
- 规模：123 个 Go 文件 / 26,251 行；40 个测试文件；21 张截图。

接手时先跑：

```powershell
$env:PATH="<go-toolchain>\bin;$env:PATH"
go build ./... ; go test ./... -count=1 ; go vet ./... ; go tool mygo vet .
go tool mygo doctor
go tool mygo build          # build/windows-amd64/食匣.exe + 安装包
pwsh -File tools/screenshot.ps1
```

## 代码地图

```text
main.go                      只做 locales/zh-Hans 导入 + app.Run()
internal/app/                窗口、壳、路由、各功能页面
  app.go                     App 状态（快照 + 各功能状态块）、View、Run（单实例锁）
  routes.go                  全部路由表（参数路由在前）
  shell.go                   侧栏、顶栏、页面骨架、stubPage
  bootstrap.go               开库 → 迁移 → 种子 → 加载各功能状态 → PhaseReady
  <feature>_views.go         各页面（pantry/recipes/sichu/schedule/feed/profile/settings/...）
  m<3-6>_*.go                对应里程碑的状态块、子页面、异步写
  lyric.go                   歌词流
  screenshot_*_test.go       各里程碑截图测试
internal/domain/             纯模型（recipe/item/plan/pantry/sichu/schedule_plan/schedule_run）
internal/data/sqlite/        唯一 import plugins/sqlite 的包（schema/迁移/仓储/settlement/...）
internal/data/seed|backup|legacy/
internal/assets/             go:embed data+images、图片 LRU、assets.lock.json
internal/design/             tokens、components kit、motion
internal/services/           matcher/schedule/nutrition/sichu/ai/season/review
tools/                       sync-assets.ps1、screenshot.ps1
```

## 硬约束（继续遵守）

- 页面不直接碰 SQLite；只走 `Store` 方法或 service。
- 领域模型不依赖 UI/DB；`services/*` 不 import `ui`/`sqlite`。
- 视图主线程逐帧；DB/网络/解码在 goroutine，`a.post`（`Window.Update`）回主线程。
- 视图里不排序/匹配/解码；944 行用 `ui.List`；派生数据在数据变化时预计算。
- 图片只经 `internal/assets.Images`（LRU + goroutine 解码）。
- 迁移 append-only；schema v1/v2 已发布，不改，新增走 v3。
- 中文字体：`main.go` 的 `locales/zh-Hans` 与 `design.FontFamily()` 不要删。
- 不整段复制 Cloudy 代码（AGPL）；不改原 Flutter 仓库。

## 已知坑（仍然成立）

- 页面转场测试要 `tt.Advance(500 * time.Millisecond)` 再断言。
- `ui.Modal` 的遮罩/面板要自己加 `Transition`；用 `design.ModalPane`。
- 整卡点击用 `ButtonBase` + `.Label()`（无障碍与测试都用它）。
- `ui.List` 才虚拟化，`ui.Grid` 会构建全部子项；列表行必须显式固定高度。
- 截图测试会重写 `test/screenshots/*.png`；提交前决定是保留新图还是还原。
- SQLite 插件首次 `Open` 需要原生库（`go tool mygo dev` 会处理；CI 需要网络）。
- 中文路径下 Go 工具链可用；NSIS 异常时用 ASCII junction。
- 本机 `-race` 不可用（cgo 未启用），由 CI 跑。
- 掌序 `hand` 资源按任务书可跨菜并行；要与 Flutter 双手互斥一致就改
  `services/schedule` 的 `capacity()`。

## 还能继续打磨的方向

1. 接 MyGo automation 插件做真实窗口截图对比（当前主证据是 `ui.Tester` 渲染）。
2. 私厨「换一道」支持一次性指令（不辣/少油），目前靠换批次。
3. 年度回顾可加菜系/时段更细的图表。
4. AI 真机联调（需要用户自备 DeepSeek Key）。
5. 并行施工用的临时 worktree 与 `feat/w1..w5` 分支已合并，可清理：

```powershell
git worktree list                 # 找到施工时创建的临时路径
git worktree remove --force <path>
git branch -d feat/w1 feat/w2 feat/w3 feat/w4 feat/w5
```
