# 食匣（FoodBox）· MyGo 原生 UI 重写计划

## Context

现有 `<FoodBox-source>` 是一个成熟的 Flutter/Dart 单机厨房应用：约 9.5 万行 `app/lib`、8.9 万行测试、Drift/SQLite v19（10 张表）、944 道菜式与 1000+ 图片资产，Android 优先、Windows 可运行、无后端无账号。

目标是在工作区**另开一个独立项目**，用 egoist 的 [MyGo](https://github.com/egoist/mygo)（MIT、纯 Go、无 cgo、GPU 绘制的原生 UI）重写这个应用。已确认：

- 界面：MyGo **原生 UI（纯 Go）**，不用 WebView / React。
- 平台：**Windows 桌面优先**，代码保持三端可编译。
- 范围：**全功能对齐**，按里程碑分批交付。
- 数据：**复用现有 JSON 种子与图片**，用 Go 重定 SQLite schema（以 Drift v19 为蓝本）；旧库可做一次性导入。

现有 Flutter 仓库不改动；新项目是独立目录、独立 git 仓库、独立 Go module。

---

## 1. 新项目与工具链

### 位置与身份

- 目录：`<repo>`
- Module：`github.com/gij072858-cmd/foodbox-go`
- 应用名：`食匣`；slug：`foodbox`；identifier：`com.gij072858.foodbox`
- Windows 显示名 `食匣`，安装目录用 ASCII `FoodBox`。
- 图标：复制 `FoodBox\_icon_out\食匣-图标-1024.png` → `FoodBox-Go\resources\icon.png`。

### 安装 Go（本机尚未安装）

```powershell
winget install GoLang.Go          # 若版本低于 1.27，改用 https://go.dev/dl/ 的 MSI
go version                        # 期望 go1.27.x windows/amd64
go env -w GOPROXY=https://goproxy.cn,direct   # 国内网络可选
```

### 脚手架（固定 MyGo v0.7.4，2026-11-01 最新 release）

```powershell
cd <projects-dir>
go run github.com/egoist/mygo/cmd/mygo@v0.7.4 init -template native `
  -name "食匣" -module github.com/gij072858-cmd/foodbox-go FoodBox-Go

cd FoodBox-Go
go get -tool github.com/egoist/mygo/cmd/mygo@v0.7.4
go get github.com/egoist/mygo@v0.7.4
go get github.com/egoist/mygo/plugins/sqlite@v0.7.4
go tool mygo doctor
go tool mygo dev
```

产物：`go tool mygo build` → `build\windows-amd64\食匣.exe` + NSIS 安装包。原生 UI 不需要 WebView2 / Bun。

### `mygo.json`

```json
{
  "name": "食匣",
  "slug": "foodbox",
  "identifier": "com.gij072858.foodbox",
  "version": "0.1.0",
  "copyright": "© 2026",
  "out": "build",
  "windows": { "displayName": "食匣", "installDirName": "FoodBox" },
  "linux": { "webview": false }
}
```

`windows.displayName` / `installDirName` / `linux.webview` 均已核对为 MyGo 合法字段。

`.gitignore`：`build/`、`.mygo/`、`*.exe`、`*.db*`、截图 actual、AI 密钥/本地配置。

---

## 2. 架构

### 目录

```text
FoodBox-Go/
├── main.go
├── mygo.json
├── resources/icon.png
├── internal/
│   ├── app/           # 窗口、壳、路由、快照、启动、命令、后台任务
│   ├── domain/        # 纯领域模型与枚举（不依赖 UI/DB）
│   ├── repository/    # 仓储接口
│   ├── data/
│   │   ├── sqlite/    # 唯一允许 import plugins/sqlite 的包
│   │   ├── seed/      # JSON 解析、首启导入、对账
│   │   ├── backup/    # 新备份格式 + 旧 v2 导入
│   │   └── legacy/    # 旧 Drift .sqlite 只读导入
│   ├── assets/        # go:embed + 图片 LRU
│   ├── design/        # tokens、Fb* 对应组件、LyricFlow、TimerCapsule
│   ├── services/      # matcher / schedule / settlement / nutrition / sichu / zhangxu / ai
│   └── features/      # pantry / recipe / schedule / sichu / feed / profile / settings / onboarding / nutrition / help
├── test/{fixtures,screenshots}
├── tools/{sync-assets.ps1,screenshot.ps1}
└── docs/
```

依赖纪律（沿用 FoodBox 现有规范）：页面不碰 SQLite；领域模型不依赖 UI/DB；功能包只调用 repository/service 接口。

### UI 状态模型

MyGo 的 `view(c *ui.Context)` 在主线程逐帧重建，事件查询式（`.Clicked()`、`ui.TextInput(c, &s)`）。因此：

- 所有模型数据放在不可变 `Snapshot`，用 `atomic.Pointer` 发布；视图只读快照，不加锁。
- 数据库读写全部在 goroutine；结果通过 `win.Update(func(){ ... })` 回主线程（已核对 MyGo 文档）。
- 任何写操作完成后显式 `Reload...()`，不做响应式流。
- 避免在视图里做排序/匹配/解码；`ui.List` 自带虚拟化，只构建可见行。
- 图片：worker 解码 WebP，`Window.Update` 内创建 `ui.Bitmap`；LRU 128–256 张，失败用分类渐变占位。
- 单实例：`mygo.App.RequestSingleInstanceLock()`，防两个进程写同一 DB。

### 启动与种子导入

1. 立即开窗显示启动页。
2. goroutine：打开 SQLite → `PRAGMA user_version` 迁移 → 读 `app_meta.seed_version` → 解析内嵌 JSON → 按名称对账 → **单事务**写入 944 菜式 / 6,930 步骤 → 同事务写 seed_version。
3. 进度通过 `Window.Update` 报告（opening / migrating / parsing / importing N/944 / ready / failed）。
4. 失败显示可读错误与重试入口，不崩溃。

---

## 3. 数据层

### Schema v1

以 `app/lib/data/local/database.dart`（Drift v19）为蓝本，`PRAGMA user_version = 1`：

- 10 张表：`items`、`recipes`、`steps`、`schedule_items`、`records`、`profiles`、`user_profiles`、`sichu_profiles`、`member_profiles`、`cook_journal`。
- 额外内部表 `app_meta(key TEXT PRIMARY KEY, value TEXT)`：seed_version、install_id、AI 用量等，不参与备份。
- ID `INTEGER PRIMARY KEY AUTOINCREMENT`；日期存 **Unix 秒 UTC**（与 Drift 默认一致）；布尔 `INTEGER`；枚举沿用 Dart 字符串名；JSON 字段 `TEXT`。
- 外键：`steps.recipe_id` / `schedule_items.recipe_id → recipes.id ON DELETE CASCADE`；`records.recipe_id` 不设 FK（记录需在删菜后保留）。
- 唯一索引：`schedule_items(recipe_id)`、`cook_journal(date)`；检索索引：`items(name)`、`recipes(name)`、`records(completed_at)`。
- 迁移链 append-only，`migrations.go` 每个版本单事务执行；库版本高于 App 时明确拒绝。

### 备份

- 新格式 `formatVersion: 3`（camelCase 表键，与 Flutter v2 对齐）。
- 可导入 Flutter v2（schema 12–19）与本项目 v3；按列名映射并补默认值；拒绝更新版本。
- 导出走 `.json.tmp` + rename；恢复前自动备份，保留 5 份；**不含 AI 密钥**。
- 旧 Drift `.sqlite`：`sqlite.Open(..., ReadOnly)` + `PRAGMA table_info` 按列名读取，单事务导入（表名/存储类型一致，成本低）。

### 资产

- 复制 `app/assets/data/*` 与 `app/assets/images/*` 到 `internal/assets/{data,images}`，`//go:embed`。
- 约 JSON 5.9 MB + 图片 28 MB；exe 约 55–70 MB，安装包约 30–45 MB，桌面端可接受。
- `tools/sync-assets.ps1` 重新同步并输出 SHA-256；`assets.lock.json` 固定哈希，CI 校验。
- 若体积不可接受：数据与分类图内嵌，条目图移到 `resources`，经 `mygo.App.Path(mygo.PathResources)` 读取。

---

## 4. 设计系统

视觉按现有 v3 口径（`app/lib/core/theme.dart`）：冷中性底、白卡、单一蓝色强调、平面分隔，不做纸张/玻璃堆叠。

- Tokens：背景 `#F1F1F1`、表面 `#FFFFFF`、分隔 `#E5E5E5`、文字 `#222222`、次文 `#8A8A8A`、品牌 `#168ACD`、警示 `#FF9500`、成功 `#57C887`；字号 11/12/14/15/17/20/28/34；间距 2–48；圆角 6–28。
- 组件映射：`FbCard`→`ui.Box` 卡片；`FbListTile`→行 + `ButtonBase`（整行可点/可聚焦）；`FbChip`/`FbSegmented`→`ui.Toggle`/`ui.Segmented`；`FbAvatar`/`FbBadge`/`FbProgress`→MyGo 同名组件；`FbBottomSheet`→底部对齐 `ui.Modal`；`FbDialog`→`ui.AlertDialog`；`FbToast`→`c.Toast` / `c.ToastAction`；`FbFoodImage`→`ui.Image` + 渐变占位；`FbGlossaryText`→`ui.RichText` 词条下划线 + 弹层；图表→`ui/chart`。
- 导航：桌面改用左侧 `ui.Sidebar`（库/菜式/私厨/掌序/食历/我的 + 设置），窄窗折叠为图标；内容最大宽 1100 居中。移动端底栏不照搬。
- 掌序「歌词流」用 `ui.Scroll` + 固定行高 + 焦点行放大/透明度 + 滚动结束吸附；悬浮计时胶囊挂在壳层，运行中每秒 `c.After` 刷新。
- 中文字体优先 `Microsoft YaHei UI, Segoe UI`；若字形/度量不理想，再用 `ui.RegisterFont` 嵌入 Noto Sans SC 子集。

---

## 5. 里程碑（全功能对齐）

每个里程碑都以「可运行 + 测试 + 截图证据」收口。

| # | 内容 | 主要来源 | 验收 |
|---|---|---|---|
| **M0** | 脚手架、tokens、侧栏壳、测试/截图框架 | `core/theme.dart` | 窗口中文正常；侧栏切页；`go test ./...`、`go tool mygo vet .` 通过 |
| **M1** | 领域模型、SQLite v1、迁移、种子导入、备份/旧库导入、启动页 | `data/models/*`、`data/local/*`、`seed_loader.dart`、`backup_service.dart` | 首启 944 菜 / 6,930 步；二次启动跳过；重复导入不重复；备份 10 表 round-trip；旧 v2 可导入 |
| **M2** | 仓库：分区、临期置顶、数量档位、详情/快捷编辑、目录添加、自建条目、盘点核销、新手包、购物清单 | `features/home/pages/pantry_*`、`starter_pack_page.dart`、`shopping_list_page.dart` | 空/有货/临期/核销四态视图测试；搜索筛选；底部全局操作条 |
| **M3** | 菜式：三维分类、现在能做/差一点/差太多、详情、编辑器、搜索、版本 | `features/home/pages/recipe_*`、`recipe_matcher.dart`、`core/recipe_taxonomy.dart` | 944 条滚动流畅；缺料 CTA 联动；保存原子并即时刷新 |
| **M4** | 掌序：待做、备料、排程引擎、歌词流执行、计时/挂起托盘、结算、会话恢复 | `features/schedule/*`、`schedule_engine.dart`、`run_settlement.dart` | 黄金时间线测试；结算失败回滚；歌词流滚动/键盘测试 |
| **M5** | 私厨 + 营养：两菜一汤/一碗端、成员与身体档案、约束、本地策略、AI、配额、营养驾驶舱 | `features/sichu/*`、`features/nutrition/*`、`services/sichu|nutrition|ai` | 本地策略确定性；结构校验；能量公式对齐 Dart 夹具；假 HTTP 测 AI |
| **M6** | 食历、做菜记录、年度回顾、我的、设置、AI Key、备份入口、引导、帮助 | `features/feed/*`、`profile/*`、`settings/*`、`onboarding/*`、`help/*` | 日记按天唯一；备份不含密钥；设置重启保持；引导只出现一次 |
| **M7** | 打包、性能、全量对齐审计 | — | 安装包可装/升级/卸载；首启导入 <10s；全清单签字 + 截图归档 |

并行：M1 为硬前置；M2/M3 可并行；M4 依赖 M2/M3；M5 的 AI/能量引擎可在 M1 后先做；M6 的设置/备份可在 M1 后先做。单人预估 12–16 周，双线 8–10 周。

---

## 6. 测试与验证

- **单元测试**：matcher、schedule engine、settlement（回滚/重复结算）、nutrition（BMR/TDEE/宏量/缺克数）、sichu（结构校验/确定性）、zhangxu 缓存、seed（944/6,930/幂等）、backup（10 表 round-trip/旧格式/拒绝新版本）、migrations。
- **无窗口视图测试**：`ui.NewTester` —— 侧栏切换、仓库四态、菜式 CTA、搜索输入、歌词流键盘/滚轮、设置表单、引导跳过、弹窗确认、无障碍角色（`tt.AccessByRole`）。
- **截图证据**：`ui.Tester.Image()` 生成 golden PNG（主）；真实窗口用 MyGo automation 插件 + `tools/screenshot.ps1`（辅）。
- **性能**：944 菜 × 200 库存匹配 <50ms；导入 <10s；列表只构建可见行；图片 LRU 不超上限。
- **CI**：`windows-latest`，Go 1.27.x，`go tool mygo vet .` + `go test ./...` + `-race`（app/services）+ `go tool mygo build -skip-installer`；校验 `assets.lock.json`。

---

## 7. 主要风险与对策

| 风险 | 对策 |
|---|---|
| MyGo 仍是 0.x，升级可能破坏 API | 固定 `v0.7.4`；所有 UI 用法收敛在 `internal/design`；只在里程碑之间评估升级 |
| 现有 Flutter 视觉/动效复杂 | 先移植 token 与信息层级，不追求像素级复刻；桌面端用侧栏替代移动底栏；截图对比验收 |
| 中文输入法与字体 | MyGo 文本输入支持 IME；每个里程碑手测微软拼音；`tt.Compose` 自动测试；字体走系统回退 |
| 图片/资产 60MB | 首版全内嵌；图片懒解码 + LRU；必要时条目图外置到 `resources` |
| 桌面无 Android | 通知/分享/相册用桌面等价物：应用内计时胶囊、`mygo.Dialog.Open/Save`、`mygo.Secrets`（Windows 凭据管理器） |
| SQLite 并发/部分写入 | 单连接 + WAL + busy timeout；种子/结算/备份走单事务；关键路径 `-race` |
| NSIS 中文路径 | slug 与安装目录用 ASCII；`mygo doctor` 先检；CI 可 `-skip-installer` |
| 旧数据兼容 | 备份 v2 映射 + Drift 只读导入 + 明确拒绝新库版本 |
| 全量重写周期长 | 严格按 M0–M7 收口；每期可运行、可截图、可回退 |

---

## 8. 审批后立即执行的 10 步

1. 安装 Go 1.27.x，验证 `go version`。
2. 用 MyGo v0.7.4 脚手架创建 `FoodBox-Go`，固定版本，跑通 `go tool mygo dev`。
3. 配置 `mygo.json`、图标、`.gitignore`，`git init` 并提交脚手架。
4. 复制 data/images 资产，写 `tools/sync-assets.ps1` + `assets.lock.json` + `//go:embed`。
5. 实现 `internal/design/tokens.go` 与带侧栏的占位壳，补第一个 `ui.NewTester` 截图测试。
6. 移植 `data/models` 领域模型与枚举，字符串值与 Dart 保持一致。
7. 写 `internal/data/sqlite` schema v1 + 迁移器 + 测试，验证 `user_version`。
8. 实现 items/recipes/steps 仓储与种子导入、启动进度/失败页。
9. 移植别名与 matcher，完成仓库只读列表，打通「DB → 视图」第一条真实数据链。
10. 移植菜式 Tab / 现在能做 / 详情，验证「库存 → 能做什么 → 加入掌序」闭环；打 `m1`/`m2` 标签，接入 CI 与 golden。

---

## 9. 关键源文件（实现时对照）

- `app/lib/data/local/database.dart` — 10 表 schema、索引、默认值、迁移历史（权威）。
- `app/lib/data/models/*.dart` — 领域模型与枚举。
- `app/lib/data/repositories/*` — 仓储契约与本地实现。
- `app/lib/data/services/recipe_matcher.dart` — 别名归一与能/差一点/差太多规则。
- `app/lib/data/services/schedule_engine.dart` — 排程时间线规则。
- `app/lib/data/services/run_settlement.dart` — 结算与库存扣减。
- `app/lib/data/services/nutrition/*`、`sichu/*`、`ai/*` — 营养、私厨、AI。
- `app/lib/core/theme.dart`、`app/lib/widgets/fb_*.dart` — 视觉 token 与组件语义。
- `docs/升级批次/功能需求清单-重做基准.md` — 功能验收口径（非视觉）。
- `docs/规范/接口约定.md`、`docs/规范/数据层使用说明.md` — 数据契约与分层纪律。

---

## 10. 计划文档落盘位置

获批后第一步把本方案复制到新项目内：`docs\方案-MyGo原生UI重写.md`，与代码一起纳入 git；后续里程碑进度另建 `docs\进度.md`。

---

## 11. 本计划的边界

- 范围已确认为**全功能对齐**；按 M0–M7 分批推进，但目标覆盖全部功能。
- 现有 Flutter 仓库、`web/`、`crawler/`、`qwenwork/` 均不改动；爬虫/录入器继续作为离线内容管线。
- 不迁移 Flutter 测试代码，按 Go 测试体系重写。
- 不承诺 Android/iOS；如需移动端，需另立方案。
- 本项目为期末作业，无商业发布计划；Cloudy 的 AGPL-3.0 不构成实际障碍，但仍以「参考思路、自行重写」为默认做法，并在文档中注明参考来源。

---

## 12. 参考实现：Cloudy（mygo-cloudmusic fork）

本地源码：`<cloudy-source>`（AGPL-3.0，145 个 Go 文件 / 约 3.05 万行）。它证明了 MyGo 原生 UI 可以承载商业级桌面应用，以下模式直接用于本项目：

| 主题 | Cloudy 文件 | 本项目的用法 |
|---|---|---|
| 三档动效速度 + ReduceMotion | `internal/app/ui_expand.go` | `design/motion.go`：跟手 spring 200ms / 弹层 280ms / 换页 340ms；设置里提供 full/lite/off |
| 微交互（hover/press/图标切换） | `internal/app/ui_kit.go` | `design/button.go`、`design/card.go` |
| 页面/弹层过渡 | `ui_expand.go` `pageMotion/paneMotion` | `app/shell.go` 路由与 `design/overlay.go` |
| 卡片→详情飞行 | `ui_expand.go` `flights/lerpRect` | 菜式卡片→详情、仓库条目→详情 |
| 歌词流 | `ui_nowplaying.go` `lyricGlide/timedLines` | 掌序执行页：一帧设偏移 + 视觉滞后，避免 ScrollTo 每帧重建 |
| 光效 | `ui_light.go` | 营养进度条、当前步骤高亮 |
| 骨架屏扫光 | `ui_skeleton.go` | 菜式/仓库加载态 |
| 自定义玻璃 shader | `ui_barglass.go` | 悬浮计时胶囊、顶部栏（轻量使用） |
| Canvas 缓存背景 | `backdrop.go` | 私厨结果页/年度回顾的氛围背景（可选） |
| 图片管线 | `artwork.go`（64MB LRU、8 并发、weak ref、失败退避） | `assets/images.go`：本地 WebP 解码与缓存 |
| 中文字体 | `fonts.go`：`_ "github.com/egoist/mygo/locales/zh-Hans"`、`Noto Sans SC, Microsoft YaHei UI` | M0 必须照做，否则 Windows 汉字会混用日文字体 |
| 帧预算测试 | `internal/app/perf_test.go` | 静止页面不得持续请求帧；动画结束即停 |
| 测试哲学 | `AGENTS.md` | 只测会坏的行为；UI 用 `ui.NewTester`；不测 MyGo 自身动画 |

有两点明确不照搬：Cloudy 没有 SQLite/迁移/种子/备份，数据层完全按本项目方案自建；它的路由是手写 history，本项目优先使用 `ui.Router`。

M0 增加「动效探针」：卡片 hover、卡片→详情飞行、底部弹层、歌词流四个交互先在 Windows 实机验证，再进入 M1。

---

## 13. 当前进度

> 2026-11-05：**M0–M7 全部完成**。逐项证据见 `docs/进度.md` 与
> `docs/验收-M7-全量审计.md`；并行施工合同见 `docs/施工-M3-M7-任务书.md`。

- [x] Go 1.27.3 + MyGo v0.7.4 原生模板；`mygo.json`、图标、SQLite 插件
- [x] M0：设计 token / 组件 kit / 侧栏壳 / 路由 / 三档动效 / 动效探针 / 中文字体
- [x] M1：领域模型、SQLite v1+v2、迁移、944 菜种子导入、备份 v3、旧 v2 导入、
      旧 Drift `.sqlite` 只读导入、启动进度/失败页
- [x] M2：仓库分区、临期、数量档位、目录、自建条目、盘点、8 地域新手包、购物清单
- [x] M3：944 列表虚拟化、三态匹配、详情、菜式编辑器、版本组、时令筛选/排序
- [x] M4：备料清单、排程引擎（灶台/案板/手法/等待 + golden）、自绘并行时间线、
      歌词流执行（键盘/暂停/悬浮胶囊/挂起托盘）、run session 恢复、
      单事务结算（records + 扣库存 + 开饭日记 + 清掌序 + XP，失败回滚）
- [x] M5：私厨（两菜一汤/一碗端、约束硬过滤、确定性本地策略、结构校验）、
      AI provider + 每日配额 + 失败回退、身体档案/成员、营养驾驶舱
- [x] M6：食历月历、开饭日记（按天唯一）、年度回顾、我的/资料/做过的菜/收藏、
      设置（动效持久化、备份、旧库导入）、AI Key、首次引导、帮助
- [x] M7：单实例锁、性能门禁（匹配 2.6 ms / 导入 146 ms / 帧预算）、
      资源哈希、CI、安装包、21 张截图、全量审计

验证：`go build ./...`、`go test ./...`、`go vet ./...`、`go tool mygo vet .`、
`go test ./internal/assets -run TestAssetsLock` 全绿；`go tool mygo build` 产出
`build/windows-amd64/食匣.exe` 与 `foodbox-setup-0.1.0-amd64.exe`。

