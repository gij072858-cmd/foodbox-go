# AGENTS.md — FoodBox-Go

本文件是给后续 AI/人类开发者的常驻约定；一次性的交接上下文见 `docs/交接-给下一个Agent.md`。

## 目标

用 Go + MyGo 原生 UI **全功能复刻**桌面版「食匣（FoodBox）」。不是精简课程版，不砍功能。
唯一边界：桌面三端（Windows 优先，macOS/Linux 保持可编译）；不要求 Android/iOS。

## 先读

1. `docs/方案-MyGo原生UI重写.md` — 架构、数据、里程碑、验收。
2. `docs/参考-Cloudy源码分析.md` — 动效/图片/字体的参考实现（允许逐字复制，见下方「许可」）。
3. `docs/学习-egoist设计手记.md` — 本轮落地的设计原则、代码入口与踩坑记录。
4. `docs/进度.md` — 当前进度。
5. 原项目：`<FoodBox-source>`（**只读，不改**）。
6. Cloudy 参考源码：`<cloudy-source>`（AGPL-3.0；可逐字复制，见「许可」）。

## 许可

本仓库是 **AGPL-3.0-only**（`LICENSE.md`），因为逐字复制了 Cloudy（AGPL-3.0）的代码单元。
对外分发必须随附对应源码。每个复制单元的文件头都要写明来源文件与行段，格式照
`internal/design/kero_theme.go` 的 Kero MIT 声明。Kero 是 MIT，可以复制但要留声明。

## 工具链

- Go 1.27.3：`<go-toolchain>\bin`（已加入用户 PATH）
- MyGo v0.7.4；`go tool mygo ...`
- GOPROXY 已设为 `https://goproxy.cn,direct`

```powershell
go test ./...
go vet ./...
go tool mygo vet .
go tool mygo doctor
go tool mygo dev
go tool mygo build
```

## 当前状态（2026-11-05）

- **M0–M7 全部完成**，逐项证据见 `docs/进度.md` 与 `docs/验收-M7-全量审计.md`。
- schema 版本 3（v3 给 `schedule_items` 加 `meal`，旧库默认 `''`）；测试覆盖 944 菜 / 6,930 步 / 幂等 / 十表备份 round-trip /
  仓库 CRUD / 三态匹配 / 排程 golden / 结算扣库存与回滚 / 私厨结构校验 /
  营养能量夹具 / AI httptest / 日记唯一 / 设置持久化 / 引导只出现一次。
- 所有页面读真实 SQLite；`ui.List` 虚拟化菜式、目录与长列表。
- 掌序：备料、并行时间线、歌词流执行、挂起托盘、会话恢复、单事务结算（扣库存+日记）。
- 私厨：本地确定性策略 + AI（DeepSeek，可配 Key，失败回退）；营养驾驶舱。
- 食历/我的/设置/引导/帮助完成；设置页有备份与 AI Key 入口。
- **M-G 对齐 egoist 原生桌面改造已完成**（2026-11-06，阶段 0–9）：44px 顶栏与面包屑、
  `ui.GridView`/`ui.Split`/`ui.Calendar`/`ui.Table`/`ui.Meter` 落地、自绘月历与圆环删除、
  单行筛选与内联计时；计数口径与证据见 `docs/进度.md` 的 M-G 段与
  `test/screenshots/{before,after}`。`internal/app` 测试耗时未达方案里的 30s（UI 帧占大头）。
- 资源哈希 `internal/assets/assets.lock.json`；CI `.github/workflows/ci.yml`。
- 原始 FoodBox 仓库未改动。并行施工用的临时 worktree 已合并，可清理
  （见 `docs/交接-给下一个Agent.md`）。

## 架构纪律

- 页面不直接碰 SQLite；只能通过 repository / service 接口。`App.store` 是
  `repository.Repository`（`Reader`/`Writer`/`Admin`），不是 `*sqlite.Store`；内存实现
  `internal/repository/memory` 只给测试。
- `internal/data/sqlite` 是唯一 import `plugins/sqlite` 的包。
- 领域模型不依赖 UI / DB。
- 视图在主线程逐帧构建；DB/网络/解码在 goroutine，结果用 `win.Update` 回主线程。
- 视图里不做排序、匹配、解码、全量构建；944 行用 `ui.List` 虚拟化。
- 图片统一走 `internal/assets` + LRU/并发上限，不在页面里直接 `DecodeBitmap`。

## 数据纪律

- schema 迁移 append-only：不要改已提交的 v1；新增表走 v2、v3……
- 日期存 Unix 秒；枚举存 Dart 原字符串；JSON 字段存 TEXT。
- `seed_version = 18`；重复导入必须幂等。
- 备份不含 AI 密钥；导入旧 v2 备份时按列名映射。

## UI / 动效

- 三档动效（完整/减弱/关闭）：`design.LevelFor` + `Level.Apply`（关闭时 `DisableAnimations`）；
  页面转场用 `Level.RouterTransition`。新增动画必须走这三档，不要只判断 `MotionFull`。
- 视觉只用 `internal/design` 的令牌：字号 `Font*`、圆角 `Radius*`、间距 `Space*`；
  页面不要再手写 9/16 这类圆角或第 7 种阴影。
- 主题色只来自 `design.PaletteFor(themeName, dark)`（Kero 十套主题，见 `internal/design/kero_theme.go`
  与 `design.KeroThemes`；算法整段复制自 Kero `theme.go`，MIT 声明必须保留，不要重写算法）。
  十套 wash 只作用于**浅色**：`PaletteFor` 在 `dark` 时一律收口到 `plain`，不要再按主题反推深色色板。
  设置页的主题入口是一个 `ui.Select`（`design.ThemeOptions` / `ThemeNameForLabel`），不给颜色预览。
  窗底用 `design.WashBackdrop` 的渐变（wash 180° + glow 225°），页面/侧栏不要再铺纯色背景；
  滚动边缘取 `p.Wash.Stops` 首/尾色；主题名持久化在 `prefs` 的 `kero_theme` 字段。
- 分段选择器：竖排容器（`Column` / `ui.Field` / `ModalPane`）里一律用 `design.Segmented`，它补上
  `AlignSelf(ui.Start)`——MyGo 的 flex 默认 cross-axis 是 Stretch，裸 `ui.Segmented` 会被拉成整卡宽、
  右边一大段无效轨道。横向 `Row` 里继续用 `ui.Segmented`（那里 Start 会变成"贴顶"）。
- 新的布局尺寸走 `design.Sp(c, n)`（主题 `Spacing` 刻度），不要手写数字；区块之间的节奏用
  `design.SectionGap` / `design.SectionLead`（Cloudy `ui_pages.go:33` 的 14/34）。`Space1..Space6`
  是 const，读不到主题，新代码别再依赖它。
- 图标统一墨色 `p.Ink`；只有画在强调色/墨色填充上的图标用 `p.OnAccent`。Accent 由
  `design.ApplyTheme` 在 `View` 每帧投影到 `ui.Theme`（按 `{Dark, Name}` 签名缓存，签名不变
  不重复 `SetTheme`），不要在页面里给图标挑强调色；`ui.Themed` 只留给设置页的主题预览。
- 悬浮/按压/阴影统一用 `design.Wash`、`design.Lift`、`RestShadow`、`HoverShadow`；
  按钮用 `design.Button` 的主/次/危险/幽灵四级；它只是 `ui.Button`/`ui.PrimaryButton`
  的薄封装，不要再用 `ButtonBase` 重写一套。
- 微交互用 `AnimatePose`（只绘制帧）；页面/弹层用 `ElementTransition`。
- 卡片→详情优先做共享元素飞行（参考 Cloudy `flights/lerpRect`），不是简单换页。
- 列表首屏用 `design.Stagger` / `StaggerRow`；骨架用 `Skeleton`；图片用 `ImageIn` / `DrawImage`。
- 反馈用框架的 `c.Toast`；`App.View` 在 `a.notice` 变化时调用一次，不要再手写内联提示。
- 能用自带组件就用自带：`ui.Table`、`ui.Tabs`、`ui.Autocomplete`、`ui.Tooltip`、
  `ui.Avatar`、`ui.PopoverMenuButton`、`ui.Progress`、`ui.Meter`、`ui.Toolbar`、
  `ui.Breadcrumbs`、`ui.Calendar`、`ui.Split` 等。
- 侧栏**不要换回 `ui.Sidebar`**：它的 `SidebarItem` 选中态是瞬切，没有 Cloudy 的滑动胶囊。
  现在的 `shell.go` 用 `ui.ButtonBase` + `ElementTransition{Match: navPillMatch{}}` 做胶囊；
  这是 MyGo 自带的 matched geometry，不是第二套动画引擎。
- 不要另画框架已有的东西：图表走 `ui/chart`（`design.BarChart` 是薄封装）；
  徽标走 `ui.Badge`（`design.Pill/Tag` 是薄封装）；分隔线用 `ui.Divider`；
  进度环弧线用 `ui.Path.Circle + Trim`；筛选 chip 用 `design.ChipTone`（底层 `ui.Toggle`）；
  弹层统一 `design.ModalDialog`（底层 `ui.DialogBase`）。新增页面不要绕过这些封装。
- 歌词流：当前行居中、滚动吸附、聚焦弹簧、当前行光晕；不要直接用 `ScrollTo` 每帧重建。
- 动效结束就停止请求帧；加帧预算测试。
- **MyGo 的 4 值 `Padding` 是 `top, right, bottom, left`**。底部间距写 `Padding(0,0,12,0)`，
  不要写成 `Padding(0,0,0,12)`（那是左边 12）。
- 中文字体：`main.go` 已导入 `locales/zh-Hans`，字体族 `Noto Sans SC, Microsoft YaHei UI`，不要删。

## 测试

- 逻辑用普通 `go test`；UI 用 `ui.NewTester`。
- 页面转场测试要 `tt.Advance(500 * time.Millisecond)` 再断言。
- 只测会坏的行为：点击/按键/滚动导致的状态变化；不测“文字显示在屏幕上”。
- 种子测试：944 道菜 / 6,930 步 / 二次导入跳过 / `PRAGMA user_version=1`。
- 每个里程碑：`go test ./...`、`go vet ./...`、`go tool mygo vet .` 全绿 + 截图。

## Git

- 每个里程碑一个 commit；不要 amend 已推送/已验收的提交。
- 提交前更新 `docs/进度.md`。
- 不提交 `build/`、`.mygo/`、`*.db*`、AI 密钥、截图 actual。
- 不要在 `main` 上直接改原始 FoodBox 仓库。

## 禁止

- 不缩范围、不删功能、不改成 WebView/React。
- 可逐字复制 Cloudy 代码；本仓库因此是 AGPL-3.0，每个复制单元必须带来源文件与行号注释（见「许可」）。
- AGPL 维护约束：`internal/design/flight.go` 与 `internal/design/gravity.go` 是从 Cloudy
  逐行复制的；保留文件头的来源文件与行号，升级 MyGo 或改这两处时同步核对上游，别让注释
  指到已经不存在的行段。
- 不修改 `<FoodBox-source>`。
- 不把 `sampleRecipes` 当成最终数据。
- 不在视图里阻塞或直接访问数据库。

## 外站视频（M-F）

- 路径：`internal/videoproxy.Resolve` 解析直链 → Windows 上交给 `video_embed_windows.go`
  创建的子 HWND（mpv `--wid`），控制走 `--input-ipc-server` 的 JSON 管道；没有 mpv、
  内嵌失败或非 Windows 平台时退回外部播放器（非 Windows）与 `video.go` 的 WebView，
  最后才 `c.OpenURL`。WebView 回退打开时自动全屏，关闭时恢复原窗口状态。
  小红书**没有**可解析接口，`Supported` 返回 false，按钮文案写「在应用内看视频」。
- 端点与参数对齐 `iawia002/lux`：Bilibili `x/player/wbi/playurl`、`fnval=16`、`qn=127`、
  必带 `Referer`。**WBI 签名已实现**（`wbi.go`：`x/web-interface/nav` 取 key、固定
  permutation、`wts`+`w_rid` MD5，`x/frontend/finger/spi` 取 `buvid3/4`）；请求带
  真实 Chrome UA、Origin、Accept 与匿名设备 Cookie，并对 `-352/-403/-412/-799` 刷新 key 重试。
- Bilibili 的 DASH 是画面/声音两条流：只有 mpv 能 `--audio-file` 同时播；VLC 必须被拒
  （`videoproxy.Launch` 里那条错误就是它），只装 VLC 的机器走 WebView。
- 真站点才暴露的坑，别改回去：`dash.video[].baseUrl` 是**字符串**（老回答是数组，
  所以 `urlList` 两种都收）；mpv 的 `--http-header-fields` 用**逗号**分隔，
  用分号会把 UA 折进 Referer，CDN 直接 403。手机分享的 `b23.tv` 短链要跟随重定向
  才拿得到 bvid 和 `?p=`（`shortBVID`）。BVID **大小写敏感**，`biliBVID` 必须原样返回匹配串。
- 内嵌播放：容器是 `STATIC` 子窗口，父窗口用 `Window.NativeHandle()`；它在 MyGo
  surface 之后创建，保持在同级 z-order 顶部。`--wid` 的窗口属于本进程，mpv 收不到
  键鼠事件，所以点击/空格/方向键/Esc 由子类化 WndProc 接住并发 IPC；`--force-window`
  必须关闭，否则又会出现独立的系统窗口。
- `SESSDATA` 只进 `mygo.Secrets`，**不进 prefsData**（备份不含密钥）。
- 「自动下载 mpv portable」未启用：mpv 官方不发 Windows 构建，只有第三方
  release 仓。别替用户选源；`portablePath(appDir)` 这个槽位已留好，
  用户自己放一份进去就生效。
