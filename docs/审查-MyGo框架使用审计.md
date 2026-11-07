# MyGo 框架使用审计报告

- **审计对象**：`<repo>`
- **框架基线**：`github.com/egoist/mygo v0.7.4`（本机模块缓存 `$(go env GOMODCACHE)\github.com\egoist\mygo@v0.7.4`）
- **审计日期**：2026-11-06
- **审计方式**：只读。对照框架源码与 `docs/ui/*.md`、项目实际调用点、`go build / go vet / go tool mygo vet / go test` 四项门禁
- **审计范围**：`internal/app`（16,113 行）、`internal/design`、`internal/data`、`internal/assets`、`internal/repository`、`go.mod`、`mygo.json`、`AGENTS.md`、`docs/*.md`

---

## 0. 三个问题的直接回答

| 用户问题 | 结论 |
|---|---|
| **有没有完全照着 MyGo 研发？** | **框架层面成立**。窗口、路由、虚拟化、控件、动画、玻璃材质、图表、拖放、位图解码、凭据库、WebView 全部是框架原语，没有自研布局引擎或渲染层。**但项目自己写的分层纪律（AGENTS.md §2）没有执行**：仓储层是死代码，页面直连 SQLite。 |
| **有无重复造轮子？** | **有 4 处真重复**（见 §3.1），其余自建组件（图片 LRU、`Flight`、`Tilt`、`Skeleton`、`Stagger`、歌词流）经框架源码核实是**框架缺口**，不算重复，见 §5。 |
| **有没有能用却没用？** | `ui` 包顶层导出（剔除测试/基准）**156 个**，项目直接引用 **64 个（41%）**，未引用 **92 个**。其中与本应用语义相关的 13 个已逐项判断，**真正应当改的 5 个**：`ui.GridView`、`ui.Sidebar`（部分）、`ui.Meter`、`ui.Themed`、`ui.Calendar`（部分）。其余 79 个是绘制底层、测试替身或本应用不存在的场景，见附录 A。 |

### 合规度评分

| 维度 | 评分 | 依据 |
|---|---|---|
| 框架驱动程度 | **A** | 127 个 `ui.*`/`mygo.*` 标识符被引用，无自研渲染/布局/事件循环 |
| 组件复用 | **B+** | 见 §2 证据表；扣分项是网格、侧栏、进度条语义 |
| 动画与动效纪律 | **A-** | 三档 `MotionOff/Lite/Full` 全覆盖，`AnimatePose`/`Match`/`Keyframes` 用在对的地方 |
| 分层架构 | **D** | `repository` 零引用，`app` 持 `*sqlite.Store`，`seed` 绕过事务封装 |
| 设计令牌一致性 | **C** | `design.Sp` 17 处 vs 两位数硬编码尺寸 228 处 |
| 无障碍 | **C-** | 全项目 `Role(...)` 仅 2 处 |
| 资源管理 | **B** | 图片 LRU + 并发上限正确，但 `ReleasePixels` 零调用 |
| 门禁健康度 | **A** | 四项门禁全绿，17 个测试包全过（4m50s） |

---

## 1. 可复现的审计命令与结果

```bash
# 环境（本机必要）
export LOCALAPPDATA="%LOCALAPPDATA%"
export GOCACHE="%LOCALAPPDATA%\go-build"
export NO_PROXY="127.0.0.1,localhost"
GO=go.exe

$GO build ./...        # BUILD_OK
$GO vet ./...          # VET_OK
$GO tool mygo vet      # exit=0，无输出
$GO test ./... -count=1
```

**测试结果：17 个包全部 `ok`，耗时 4m50s**，`internal/app` 单包 278s。
`internal/repository` 显示 `[no test files]`——**这是本报告 P1-1 的直接证据：这个包没有任何测试，也没有任何调用者。**

> 注意：不带 `LOCALAPPDATA` 直接跑 `go test ./...` 会出现 60+ 个 `%LocalAppData% is not defined` 失败。这是环境变量缺失，不是代码缺陷；`go test` 在正常 Windows 会话（有该变量）下是绿的。

框架能力盘点方法（可复现）：

```bash
cd "$(go env GOMODCACHE)/github.com/egoist/mygo@v0.7.4/ui"
grep -hoE "^func [A-Z][A-Za-z0-9_]*\(" *.go | sed 's/^func //;s/(//' | sort -u \
  | grep -vE "^(Test|Benchmark)" > /tmp/all.txt          # 156
cd "<repo>"
grep -rhoE "ui\.[A-Z][A-Za-z0-9_]*" --include=*.go internal \
  | sed 's/^ui\.//' | sort -u > /tmp/used.txt            # 127（含方法与类型）
comm -23 /tmp/all.txt /tmp/used.txt                      # 92
```

---

## 2. 合规证据：这些地方确实是「照 MyGo 做的」

这一节是对「完全由 MyGo 驱动」的正面核实。每一条都能在框架源码里找到对应实现，不是靠命名猜的。

### 2.1 应用与窗口

| 能力 | MyGo API | 项目位置 |
|---|---|---|
| 应用入口 | `mygo.App.Run()` | `main.go`、`app.go:279` |
| 中文本地化 | `mygo.App.SetLanguages("zh-Hans")` | `app.go:243` |
| 单实例锁 | `RequestSingleInstanceLock` + `OnSecondInstance` | `app.go:250-258` |
| 退出钩子 | `mygo.App.OnQuit` | `app.go:262` |
| 就绪回调 | `mygo.App.WhenReady` | `app.go:265` |
| 自绘标题栏窗口 | `mygo.NewWindow(WindowOptions{TitleBarStyle: TitleBarHidden, TitleBarHeight})` | `app.go:268-277` |
| 标题栏拖拽区 | `c.TitleBar()` + `.DragWindow()` | `shell.go:170-175` |
| 应用数据目录 | `mygo.App.Path(mygo.PathUserData)` | `bootstrap.go:31` |
| 跨 goroutine 刷新 | `win.Update(fn)` 收敛到 `a.post()` | `app.go:400-407` |
| 原生 WebView | `win.NewWebView(...)` + `ui.WebView(c, v)` | `video.go:138`、`video.go:190` |
| 系统凭据库 | `mygo.Secrets.Get/Set/Delete` | `video.go:209/223/228` |

**没有** 自建窗口、自建事件循环、自建平台抽象层。这是「原生」最硬的一条证据。

### 2.2 路由与转场

| 能力 | MyGo API | 项目位置 |
|---|---|---|
| 路由表 | `ui.Router` + `r.Match("/pantry/{}")` + `r.Title()` | `routes.go:34-120` |
| 转场档位 | `RouterTransition()` → `TransitionNone/Fade/Auto` | `design/motion.go:71-79` |
| 飞行中降级 | 飞行时强制 `TransitionNone` | `app.go:330-340` |

`routes.go` 里那个手写 `switch` 是**路由表**（模式 → 页面函数），不是路由实现；匹配本身由 `Router.Match` 完成，且注释明确写了「具体模式必须排在参数化模式之前」——这是理解框架匹配语义后的写法。

### 2.3 虚拟化与大数据量

| 能力 | MyGo API | 项目位置 | 备注 |
|---|---|---|---|
| 按行虚拟化 | `ui.List(c.Key, state, rows, row)` | `shell.go:275` | 944 条菜式只用它 |
| 自适应网格 | `ui.GridView(c.Key, state, n, minW, rowH, row)` | `recipes_views.go:269` | 菜式页 |
| 表格 | `ui.Table` + 排序/选块 | `m5_nutrition.go:485-511` | 营养摄入表 |
| 滚动跟踪 | `Scroll(...).TrackScroll(s)` | `shell.go:228` | |
| 滚动边缘材质 | `glass.ScrollEdge` | `shell.go:269` | 没手绘渐变 |

`perf_test.go:108` 的 `TestStaticPageRequestsNoFrames` 用 `tt.FramePending()` 断言静止页面不再请求帧——**这是框架帧模型被真正理解并写进门禁的证据**，比任何注释都有说服力。

### 2.4 控件族（事件查询式）

框架的 base 系列（逻辑/可达性由框架给，外观自己画）在项目里被大量复用的证据：

```
ui.ButtonBase        shell.go:124（侧栏项）
ui.Toggle            design/components.go:147（Chip）
ui.Segmented         settings_views.go:36/58、m5_nutrition.go:471
ui.Tabs              schedule_views.go:39
ui.Collapsible       settings_views.go:26
ui.Accordion         m6_help.go:108-126
ui.Select            m5_sichu.go:1061/1065/1069
ui.Combobox          m3_recipe_editor.go:578
ui.Autocomplete      m3_recipe_editor.go:634
ui.NumberInput       m3_recipe_editor.go:550/553/637/643/689
ui.StepSlider        m5_sichu.go:1055
ui.DateInput         feed_views.go:51
ui.SearchField       m6_profile.go:745
ui.FindBar           pantry_views.go:554
ui.Breadcrumbs       recipes_views.go:444/514
ui.PopoverMenuButton schedule_views.go:707
ui.DialogBase        design/components.go（ModalDialog）、video.go:173
ui.AlertDialog       m6_profile.go:682、m3_recipe_editor.go:815
ui.Badge             design/components.go（Pill/Tag）
ui.Avatar            profile_views.go:95、m5_sichu.go:1274
ui.Meter             m5_nutrition.go:458（嘌呤负荷，用法完全正确）
ui.Progress          startup.go:43（启动进度，用法正确）
ui.Fieldset          m3_recipe_editor.go:470
ui.Split             m3_recipe_editor.go:400
ui.Grid              pantry_views.go:647（新手包静态 4 列）
```

### 2.5 动画

| 能力 | MyGo API | 项目位置 |
|---|---|---|
| 布局动画 | `.AnimateTo(key, target, dur)` | `lyric.go:61` |
| 颜色动画 | `.AnimateColor(...)` | `design/components.go`（Wash） |
| 只重绘动画 | `.AnimatePose(...)` | `design/components.go`（Lift） |
| 关键帧 | `.Keyframes("intro", ...)` | `startup.go:19-25` |
| 匹配几何 | `ElementTransition{Match}` | `shell.go:160`（滑动胶囊） |
| 数字滚动 | `ContentTransition{Numeric:true}` | 统计数字 |
| 三档语义 | `MotionOff/Lite/Full` + `c.DisableAnimations()` | `design/motion.go:12-68` |
| 减动效 | `Preferences().ReduceMotion` 优先 | `design/motion.go:45-53` |

**`AnimatePose` 与 `AnimateTo` 的分工是对的**：`Lift`（悬停抬起）只改 transform，用 pose；`Wash` 只改颜色，用 `AnimateColor`。`docs/学习-egoist设计手记.md` 里「能 pose 就不 layout」这条纪律在代码里被执行了，不是写在文档里好看的。

### 2.6 绘制与图形

| 能力 | MyGo API | 项目位置 |
|---|---|---|
| 图表 | `ui/chart` 的 `NewBand/NewLinear/Plot/Axes/Rect` | `design/components.go`（BarChart） |
| 环形进度 | `Path.Circle().Trim(0, shown)` + `pnt.StrokePath` | `design/components.go`（Ring） |
| 位图解码 | `ui.DecodeBitmap`（含 EXIF 转正、WebP/BMP） | `internal/assets/images.go` |
| 玻璃材质 | `glass.Glass{Tint}` | `design/components.go`（PaneMaterial/BarMaterial） |
| 着色器 | `shader.MustCompile` | `design/gravity.go:57` |
| 拖放 | `handle.Drag` + `Dragged()` + `ui.Drop` / `DragOver` | `lyric.go`、`schedule_views.go:470/487` |

### 2.7 数据与并发

| 能力 | MyGo API | 项目位置 |
|---|---|---|
| SQLite 插件 | `plugins/sqlite`（`Open/BusyTimeout/Transaction/Query/Execute`） | `data/sqlite/db.go` |
| WAL + 忙等 | `PRAGMA journal_mode=WAL`、`BusyTimeout 5s` | `db.go:28-40` |
| 单事务批量写 | `Transaction` + `[]Statement` | `seed.go`、`recipes_write` |
| 图片后台解码 | goroutine + `sem chan struct{}`（4 并发） | `assets/images.go` |
| UI 测试 | `ui.NewTester` **77 处** | 各 `*_test.go` |

---

## 3. 问题清单

严重度定义：
**P0** = 已经或即将导致数据口径分叉 / 并发缺陷；**P1** = 破坏项目自定契约、阻碍演进；**P2** = 一致性、清洁度、可访问性。

### 3.1 真·重复造轮子（4 处）

#### P0-1 `nutritionReadProfile` 与 `GetSichuProfile` 是同一行的两种读法，口径已分叉

**证据**：

`internal/app/m5_nutrition.go:137-166` 手写：
```go
func nutritionReadProfile(ctx context.Context, store *sqlite.Store) (domain.SichuProfile, bool, error)
    rows, err := store.Query(ctx, `SELECT height_cm, weight_kg, age, sex,
        activity_level, target_weight_kg, body_fat_pct, life_stage,
        nutrition_goal, nutrition_goals, calorie_target, health_stages
        FROM sichu_profiles WHERE id = 1`)
```
只读 **12 列**。

`internal/data/sqlite/sichu.go:22-70` 已有完整实现：
```go
const sichuProfileColumns = `height_cm, weight_kg, age, sex, activity_level,
    target_weight_kg, body_fat_pct, life_stage, allergens, health_stages,
    health_tags, health_note, nutrition_goal, nutrition_goals, calorie_target,
    restrictions, allergy_note, explore_ratio, target_meals`
```
读 **19 列**。

**已经分叉的四处**：

1. **漏列 7 个**：`allergens`、`health_tags`、`health_note`、`restrictions`、`allergy_note`、`explore_ratio`、`target_meals`。目前 `nutritionEnergyInput()`（`m5_nutrition.go:169`）与 `nutritionGoutAcute()`（`:248`）恰好没用到这 7 列，所以**还没有可见 bug**——但任何一次「营养页显示忌口」的迭代都会静默读到空串。
2. **缺行的语义相反**：`GetSichuProfile` 在无行时返回默认值并夹取 `ExploreRatio`（`sichu.go:41-43, 66-68`）；`nutritionReadProfile` 返回 `hasProfile=false`。两条路径对「新库」的判断不同。
3. **类型转换写了两套**：项目有 `asInt/asString/asFloat`（`sichu.go`），又在 `m5_nutrition.go:594-622` 写了 `nutritionInt/nutritionString/nutritionFloat`。注释还写着「without importing sqlite's helpers」——说明作者知道重复，但选择了复制。
4. **解耦方式反了**：`m5_nutrition.go:73` 用 `var nutritionProfileReader = nutritionReadProfile` 拿到一个测试缝。为了这个缝，复制了整段读库逻辑；正确的缝应该开在 `Store` 上（见 P1-1）。

**影响面**：营养页、私厨页、成员卡三处都以 `SichuProfile` 为准，现在有两个真源。

**修复**：删 `nutritionReadProfile` + 三个 helper，改调 `store.GetSichuProfile(ctx)`，`hasProfile` 由 `rows == 0` 判断（或让 `GetSichuProfile` 返回 `(p, ok, err)`）。测试缝改成注入 `profileReader func(ctx) (domain.SichuProfile, bool, error)` 的接口，而不是复制实现。

---

#### P1-2 手写网格替代 `ui.GridView`（两处）

**证据**：

```go
// pantry_views.go:116，仓库页 3 列
rows := (len(a.pantryView) + 2) / 3
a.listBody(c, "pantry-list", &a.pantryList, rows, func(i int) {
    row := ui.Row(c)...Height(174)...
    for _, j := range []int{i*3, i*3+1, i*3+2} { ... a.pantryCard(...) }
})

// pantry_views.go:566，目录页 2 列
rows := (len(a.catalogView) + 1) / 2
```

而同项目的菜式页用的是框架组件：
```go
// recipes_views.go:269
ui.GridView(c.Key("recipe-grid"), &a.recipeGrid, len(a.recipeView), 360, 76, ...)
```

**为什么是重复**：`ui/gridview.go:89-148` 做两件手写版做不到的事——
```go
cols := max(1, int(math.Floor((width+gap)/(minWidth+gap))))
// 列数变化时：ScrollTo(first * s.cols / cols)   // 按同一批 item 重新锚定
```
手写版**列数写死**（3 和 2），窗口变窄时不会降列，只能横向溢出；滚动位置也不会在重排后保位。

**影响面**：仓库页与目录页在 1366×768 及更窄窗口下体验劣化；同项目内两套网格写法，改一处容易漏另一处。

**修复**：两处换 `ui.GridView`，`minWidth` 分别取 300/420 左右，原有卡片函数直接做 row builder。

---

#### P1-3 自建月历（有理据，但可以更省）

**证据**：`feed_views.go:32-79` 手写 `上月/下月/本月` 按钮 + 星期头 + `for week := range f.weeks` 双层循环 + `feedCell`；`ui.Calendar` 未使用。

**判断**：`ui.Calendar` 的单元格只能放日期，而食历格要显示「当天几道菜 + 菜名首字」——**这是框架能力的真实缺口，不算重复造轮子**。

**仍可借的部分**：月份导航的键盘语义、星期头的本地化首日（周一起始）、`RoleGrid/RoleGridCell` 的可访问性。目前自建版的格子没有任何 Role，读屏器读不到「10 月 3 日，做了 2 道菜」。

**修复（低优先级）**：保留自绘格，但补 `Role(RoleGridCell)` + `.Label("10月3日，2 道菜")`；月份切换按钮改用 `ui.IconButton`（当前是自绘）。

---

#### P1-4 `ui.Progress` 被当作「水平占比条」用在排行与构成比（语义错位）

**证据**：
```go
feed_views.go:362   ui.Progress(c, float64(fraction))   // 最常做排行
feed_views.go:385   ui.Progress(c, float64(fraction))   // 菜系占比
m6_profile.go:725   ui.Progress(c, float64(row.Count)/float64(top))
recipe_hero.go:113-117  自绘 4px 轨道 + Grow(frac) 填充
```

**框架语义**（`ui/widgets.go:460`）：
> `Progress` creates a progress bar filled to value between 0 and 1 … `e.role = RoleProgress`

**框架等价物**（`ui/indicators.go:51-56`）：
> `Meter` creates a bar showing value between lo and hi, as AppKit's level indicator and the web's meter … Assistive technology sees a **level indicator** of that value; name it with `Label`.

**影响**：读屏器会把「川菜占 23%」念成「进度 23%」，把静态构成比误报成正在进行的任务。这是**语义 bug**，不是审美问题。

**修复**：四处换 `ui.Meter(c, v, 0, 1, nil).Label("川菜占比")`；`recipe_hero.go:113-117` 的自绘条同样换 `ui.Meter(...).Height(4)`。项目里 `m5_nutrition.go:458` 已经正确用过 `ui.Meter`，说明这个 API 团队是熟的，只是没统一。

---

### 3.2 分层契约：项目自己定的规则没执行

#### P1-1 仓储层是死代码，页面直连 `*sqlite.Store`

**证据**：
```
internal/repository/interfaces.go  定义了 7 个接口（Recipe/RecipeWrite/Item/Schedule/Journal/Profile/Sichu）
grep -rn "repository" --include=*.go internal | grep -v internal/repository/   → 0 命中
```
而实际链路是：
```go
app.go:34        store *sqlite.Store
state.go:25      func (a *App) loadFeatureState(ctx context.Context, store *sqlite.Store) error
```
`internal/app` 里直接出现 `sqlite.Store` 的**20 处**。

**这直接违反项目自己的 AGENTS.md：**
```
AGENTS.md:56  - 页面不直接碰 SQLite；只能通过 repository / service 接口。
```

`docs/方案-MyGo原生UI重写.md` §2 也写了同一件事。

**为什么这是最该修的一条**：
1. **测试只能起真库**。缺了仓储接口，`nutritionReadProfile` 才不得不复制一份（P0-1）——两个问题是同一个根因。77 处 `ui.NewTester` 里有大量 `newTestStore(t)`，每个测试都付 sqlite 打开 + 迁移 + 建表的成本，`internal/app` 单包测试跑 **278 秒**。
2. **不可替换**。想加一层内存缓存、想换 FTS5 索引、想在写入前做校验，都要改视图文件。
3. **契约漂移无人拦**。`repository/interfaces.go` 的方法签名没人调用，等于没有编译期保护；它现在是一份「看起来分层了」的装饰。

**为什么当初会这样**：`App` 是唯一持有依赖的地方，直接把具体类型传下去最省事；等到想收口时，20 个调用点已经铺开了。

**修复（见 §4 批次 3）**：不要一次改 20 处。先把 `repository` 里 7 个接口合并成 `App` 真正需要的 3 个（`Reader`/`Writer`/`Admin`），再用「`*sqlite.Store` 满足接口」这一事实做零改动替换——`App.store` 字段类型从 `*sqlite.Store` 改成 `Repository`，所有 `store.Xxx()` 调用点**一行都不用改**。这一步做完，`nutritionReadProfile` 才有地方消失。

---

#### P1-5 `seed` 绕过事务封装，`Store.DB()` 把裸连接漏出去

**证据**：
```go
// seed.go:9
msqlite "github.com/egoist/mygo/plugins/sqlite"

// seed.go:46
func Import(ctx context.Context, db *msqlite.DB, progress func(Progress)) (Summary, error)

// db.go:50
func (s *Store) DB() *msqlite.DB { return s.db }
```

**违反**：
```
AGENTS.md:57  - `internal/data/sqlite` 是唯一 import `plugins/sqlite` 的包。
```

**为什么是问题**：`Store.Run([]Statement)` 已经把「一次 Import = 一个事务」封装好了（`db.go`），`seed` 却拿裸 `*msqlite.DB` 自己攒 `[]msqlite.Statement`。后果是：
- 事务边界不再由 `sqlite` 包统一裁决，未来若给 `Run` 加「写前校验/审计/批量上限」，seed 会静默绕过；
- `DB()` 是一个公开的逃生口，任何人拿到它就能绕过 `BusyTimeout`、绕过 `Run`、绕过未来的读写分离。

**修复**：`Import(ctx, store *sqlite.Store, progress)`，内部改用 `store.Run(...)`；`DB()` 若只剩测试用，改名 `DBForTest()` 或直接删除（检查 3 个调用点后删除是可行的）。

---

### 3.3 主题与令牌

#### P1-6 主题双轨：`design.Palette` 与 `ui.Theme` 并行，每帧 `c.SetTheme` 重建整窗

**证据**：
```go
// theme.go:22-60，每帧在 View 里调用
func ApplyTheme(c *ui.Context, mode, themeName string) *ui.Theme {
    t := *c.Theme()
    ... 20 多行字段覆盖 ...
    c.SetTheme(&t)          // theme.go:59
    return &t
}
```
`design.Palette` 被引用 **165 处**，是页面取色的真实来源；`ui.Theme` 只是它的投影。

**两个具体问题**：

1. **`ui.Themed` 零使用，但 AGENTS.md 说用了它**：
```
AGENTS.md:89  - Accent 由 `shell.go` 的 `ui.Themed` 覆盖，不要在页面里给图标挑强调色。
AGENTS.md:98  - `ui.Avatar`、`ui.PopoverMenuButton`、`ui.Progress`、`ui.Themed` 等。
```
`grep -rn "ui.Themed" --include=*.go internal` → **0 命中**。文档与代码不一致，下一个接手的人会照着文档去找一个不存在的调用。

2. **每帧 `c.SetTheme(&t)`**：`SetTheme` 的语义是「整窗换主题」（`ui/context.go:342-408`），会作用于所有子树与浮层。项目每帧都重设一遍同样的值——功能上没错（值稳定时框架缓存得住），但**语义上放弃了 `ui.Themed` 提供的局部作用域**：如果哪天要做「设置页预览主题（其它页面不变）」，现在这套结构做不到。

**修复（二选一，别两件都做）**：
- **A（推荐，成本低）**：承认 `Palette` 是唯一真源，`ApplyTheme` 保留；把 AGENTS.md:89/98 改成「Accent 由 `design.ApplyTheme` 每帧投影到 `ui.Theme`」，删掉 `ui.Themed` 的错误描述。
- **B（成本高，收益中）**：`ApplyTheme` 只在主题/明暗真正变化时执行（用一个 `themeSig{Dark, Name}` 比较），其余帧用 `ui.Themed(c, t)` 只包需要覆盖的子树。

**先做 A**。文档说错话比代码绕路更贵。

---

#### P2-1 设计令牌被系统性绕过

**证据**（`internal/app` 内统计）：

| 指标 | 数量 |
|---|---|
| `design.Sp(c, n)` 调用 | **17** |
| 直接写两位以上数字的 `.Padding/.Gap/.Width/.Height/.MinWidth/...` | **228** |
| `.FontSize(<数字>)` | **242** |
| `.Radius(<数字>)` | **56** |

分布最密的文件：`m5_sichu.go` 34、`schedule_views.go` 29、`m3_recipe_editor.go` 27、`pantry_views.go` 26、`feed_views.go` 19。

**违反**：
```
AGENTS.md:74  - 视觉只用 `internal/design` 的令牌：字号 `Font*`、圆角 `Radius*`、间距 `Space*`；
AGENTS.md:85  - 新的布局尺寸走 `design.Sp(c, n)`（主题 `Spacing` 刻度），不要手写数字；
```

`tokens.go:60` 的 `Sp` 注释写得很清楚：「the consts above cannot do this — a const cannot read the theme — so new layout code goes through Sp」。**规则本身是对的，只是没落地。**

**后果**：`ui.Theme.Spacing`（当前固定 4，`theme.go:54`）这个「紧凑/宽松」杠杆对 90% 的界面无效——改它只会让那 17 处动，视觉立刻撕裂。

**修复**：不要全量替换（228 处是半个月的活且容易引入回归）。只做**新代码强制 + Top3 文件收敛**（见 §4 批次 4）。

---

### 3.4 资源与性能

#### P1-7 `ReleasePixels` 零调用，96 张位图的 CPU 像素常驻

**证据**：`grep -rn "ReleasePixels" internal` → 0 命中；`app.go:221` `assets.NewImages(96)`。

**框架建议**（`ui/bitmap.go:127-160`）：位图首次显示后可调 `ReleasePixels()`，在主线程上释放 CPU 侧像素，GPU 纹理保留——对图谱类应用是纯收益。

**估算**：菜式封面多为 800×600，`RGBA` 4 字节 → 约 1.9 MB/张 × 96 ≈ **180 MB** 常驻，仅为了偶尔的重绘。

**修复**：在 `assets.Images` 的 `Get` 路径里，图片「已经从缓存取出并至少显示过一帧」后调 `ReleasePixels`。注意必须在主线程、且之后 `DecodeBitmap` 会被重新需要时能重建（框架文档说明 `ReleasePixels` 后仍可正常绘制与缩放，只是再次取像素要重新解码）。

#### P2-2 `NewImages(96)` 低于方案文档的 128–256

`docs/方案-MyGo原生UI重写.md` 写 128–256，代码是 96。要么改代码要么改文档——现在两边不一致，且 96 在「仓库页 + 菜式页来回切」时会有可感知的重新解码。

---

### 3.5 死代码

#### P2-3 `design` 包 7 个导出无人使用

```
design.Tile        design.ModalPane    design.IconButton
design.FadeImage   design.OverlayTint   design.Fragment   design.FontFamily
```
（`ModalPane`、`OverlayTint` 仅在 `design` 包内被别的函数用到 → 应改小写，不该导出。）

`internal/repository/interfaces.go` 整包是死代码（P1-1）。

#### P2-4 手写列表/面板缺 Role

全项目 `Role(...)` 仅 **2 处**，且都在自建侧栏（`shell.go:73` `RoleTree`、`shell.go:125` `RoleTreeItem`）。

缺的地方：
- `m6_profile.go:661-680 / 757-810` 的手写列表面板（循环 `ui.Row`，无 Role，也不虚拟化）；
- 食历月历格（P1-3）；
- 仓库页/目录页的手写网格（P1-2）。

对照：`ui.GridView`、`ui.Table`、`ui.List` **免费给出** `RoleGrid/RoleGridCell`、`RoleTable`、`RoleList/RoleListItem`。**换组件顺手就修好了无障碍**——这也是 P1-2 优先级高于外观收益的原因。

---

### 3.6 许可证与外部依赖

#### P1-8 AGPL 代码复制带来的升级成本

**证据**（两处头注释都自认）：
```
flight.go:1-3   Copied from github.com/nya-owo/cloudy internal/app/ui_expand.go …
                Cloudy is AGPL-3.0-only, Copyright (c) EGOIST.
gravity.go:1-2  Copied from github.com/nya-owo/cloudy internal/app/ui_cover.go.
```
`LICENSE.md` 确实是 AGPL-3.0。

**判断**：这是**知情选择**，不是疏忽——头注释精确到行号（`ui_stage.go:46`、`ui_expand.go:50-53`…）。但代价要写进文档：

1. 项目整体被锁定为 AGPL-3.0，**不能再改成 MIT/闭源分发**；
2. Cloudy 上游修复 bug 时不会自动流过来，需要人工跟；
3. 两段代码都是**框架能力缺口**的补位（`Match` 不能跨页面飞、框架无 3D 倾斜），未来 MyGo 若补上，应优先删除自建版本。

**建议**：在 `AGENTS.md` 里加一条「`design/flight.go`、`design/gravity.go` 是外部 AGPL 代码，修改时保留头注释里的行号引用，上游变更需同步」。

#### P2-5 mpv 外挂播放与「纯 Go 单机」口径的张力

`video.go:68-115` 用 `exec.Command` 起 mpv。框架侧原因是客观的（`video.go:28-30` 注释：`ui.Video` 只解 animated GIF，MP4 走不了）。但产品口径说「离线单机、数据只在本机」，实际上多了一个**外部可执行文件依赖**——用户机器上没有 mpv 时这条路径会静默失败。

**建议**：不是要改实现，而是要在设置页/帮助页把「视频预览需要本机安装 mpv」说清楚，或在缺失时给明确错误，而不是黑屏。

---

## 4. 优化方案（4 个批次，可独立交付）

每批次都能单独提交、单独验收，不要求一次做完。

### 批次 1 · 一天内，零行为风险

| 改动 | 文件 | 验收 |
|---|---|---|
| 删 7 个 design 死代码 | `design/components.go` 等 | `go build ./...`；`go test ./internal/design` |
| `Progress` → `Meter`（4 处） | `feed_views.go:362/385`、`m6_profile.go:725`、`recipe_hero.go:113` | 截图比对 `test/screenshots/`；读屏器不再念「进度」 |
| `ReleasePixels` 接入 | `assets/images.go` | 内存面板 RSS 下降；`go test ./internal/assets` |
| `NewImages(96)` → `160` | `app.go:221` | 与方案文档对齐 |
| AGENTS.md:89/98 修正 | `AGENTS.md` | 文档与代码一致 |

**预期收益**：~180 MB 常驻内存、1 个语义 bug、7 个死符号、1 处文档谎言。**不碰任何业务逻辑。**

### 批次 2 · 1–2 天，消灭分叉

| 改动 | 说明 |
|---|---|
| 删 `nutritionReadProfile` + `nutritionInt/nutritionString/nutritionFloat` | 改调 `store.GetSichuProfile`；`hasProfile` 改成 `(p, ok, err)` 三返回值 |
| `seed.Import` 收口 | 签名改 `(ctx, store *sqlite.Store, progress)`，内部走 `store.Run` |
| `Store.DB()` 删除或收窄 | 先查 3 个调用点，能删就删 |

**验收**：`go test ./internal/app ./internal/data/... -count=1` 全绿；`grep -rn "plugins/sqlite" internal` 只剩 `internal/data/sqlite`。

### 批次 3 · 2–3 天，仓储层真正落地

**关键技巧：零改动替换。** `*sqlite.Store` 天然满足接口，所以：

```go
// 之前
type App struct { store *sqlite.Store }
func (a *App) loadFeatureState(ctx context.Context, store *sqlite.Store) error

// 之后（唯一改动是类型）
type Repository interface {
    ListAll(ctx) ([]domain.RecipeBrief, error)
    ...  // 从 repository/interfaces.go 里挑 App 真正用到的
}
type App struct { store Repository }
func (a *App) loadFeatureState(ctx context.Context, store Repository) error
```

**20 个 `store.Xxx()` 调用点一行都不用改**——因为方法集是 `*sqlite.Store` 的子集。`internal/repository/interfaces.go` 里那 7 个接口先合并成 3 个：`Reader`（读）、`Writer`（写）、`Admin`（迁移/备份/seed），其余删掉。

做完这一步，`nutritionProfileReader` 才有地方落成真正的测试替身（内存实现），`internal/app` 测试可以从 278s 降到十几秒。

**验收**：`go build ./...`；`internal/app` 里 `sqlite.Store` 出现次数 = 0（只在 `bootstrap` 里构造一次）。

### 批次 4 · 3–5 天，组件与令牌收敛

| 改动 | 位置 | 收益 |
|---|---|---|
| 手写网格 → `ui.GridView` | `pantry_views.go:116,566` | 窄窗降列 + 重排保位 + 免费 Role |
| 月历补 `RoleGridCell` + `Label` | `feed_views.go:83-116` | 读屏可达 |
| 令牌收敛（只做 Top3） | `m5_sichu.go`、`schedule_views.go`、`m3_recipe_editor.go` | `Spacing` 杠杆开始生效 |
| 侧栏补跨组箭头 + 首字母跳转 | `shell.go:106-166` | 与 `ui.Sidebar` 键盘行为对齐 |

**侧栏要不要换 `ui.Sidebar`？** 我的判断：**不换**。理由见 §5。

---

## 5. 不建议改动的部分（附理由）

审计报告最容易犯的错是「凡是自建的建议换成框架」。以下 7 处经源码核实，**自建是对的**：

| 自建 | 框架对应 | 为什么不换 |
|---|---|---|
| `internal/assets` 图片 LRU | **无** | `ui` 包只有 `DecodeBitmap`/`DecodeImage`/`ImageSize`，**没有解码位图缓存**。LRU + 4 并发信号量是必需品，头注释的声明准确。 |
| `design.Flight`（跨页飞行） | `ElementTransition{Match}` | `Match` 只在同一棵元素树内生效，跨路由页面的共享元素飞行框架不支持（`app.go:330-340` 飞行时强制 `TransitionNone` 就是证据）。 |
| `design.Tilt`（3D 倾斜） | **无** | 框架没有 `RotationX/Y` 之外的指针跟随弹簧；`Perspective/RotationX/RotationY` 原语存在但没有 `hover→spring→transform` 的组合件。 |
| `design.Skeleton` | **无** | 框架无 Skeleton 组件；`docs/ui/motion.md` 明确要求自己 `AnimationFrameRate(60)`。 |
| `design.Stagger` | **无** | 框架无 stagger 容器；项目按手记上限 14 行、24ms 步长，符合设计约束。 |
| 歌词流 | **无** | 业务特有（逐行高亮 + 居中偏移 + 拖拽排序）。 |
| 自建侧栏 | `ui.Sidebar` | `SidebarItem` 的外观是框架写死的（`sidebar.go:160-200`），**无法承载滑动胶囊**。项目用 `Match` 让胶囊飞过去，`shell.go:69-72` 的注释把取舍写清楚了——这是**用框架原语做的正确取舍**，不是重造动画引擎。代价是失去 `Sidebar` 的跨组箭头、首字母跳转、`SidebarSection` 折叠，建议**补键盘行为**而不是换组件。 |
| `ui.Progress` 之外的 `ModalDialog` | `ui.Modal` | `ui.Modal` 用不透明白底，玻璃材质读不到背后页面（`学习手记` 172-174 记录了这个坑），`DialogBase` + 自绘面板是**框架推荐的自定义 Modal 路径**。 |
| mpv 外挂 | `ui.Video` | `Video` 只解 animated GIF，MP4/WebM 不支持。这是框架真实限制。 |
| `sichuMapChip` 循环 `ui.Toggle` | `ui.ToggleGroup` / `ui.CheckboxGroup` | `ToggleGroup` 是**拼接式分段控件**（`toggle.go:66-78`，segmentTrack），`CheckboxGroup` 是**全选父 + 缩进子项**（`feedback.go:12-23`）。两者外观都不是 chip 流，硬套会变形。手写循环 `ui.Toggle` 是正确用法。 |

---

## 6. 附录 A：92 个未使用的 `ui` 导出一览（已分类）

**A. 与应用语义相关，需逐项判断（13 个）**

| 标识符 | 判断 |
|---|---|
| `Calendar` | 有理据不用（格内要放富内容），但可借键盘/星期头语义 → §3.1 P1-3 |
| `SidebarItem` / `SidebarSection` | 有理据不用（胶囊外观）→ §5 |
| `GridView` | **应用而未用** → P1-2 |
| `Meter` | **应用而未用**（除 `m5_nutrition.go:458` 一处）→ P1-4 |
| `Themed` | **应用而未用**，且文档声称用了 → P1-6 |
| `Modal` | 有理据不用（玻璃）→ §5 |
| `Video` | 有理据不用（仅 GIF）→ §5 |
| `ToggleGroup` / `CheckboxGroup` | 外观不匹配 → §5 |
| `Popover` / `Overlay` | 由 `PopoverMenuButton`/`DialogBase` 覆盖，无需直接用 |
| `Toolbar` | 自绘标题栏设计，不适用 |
| `Rating` | 项目无评分功能 |
| `Spinner` | 用 Skeleton 更合适（框架两者都无强制） |

**B. 绘制/变换底层，正常不需要直接用（约 40 个）**
`Shape` `ShapeText` `ShapeRichText` `Path` 系列辅助、`Perspective` `RotationX/Y` `Skewing` `Scaling` `Project` `Rubberband` `Nearest` `Linear` `FitContent` `Fixed` `RGB` `Fr` `ScrollBoth` `SplitPercent` `SplitVertical` `EaseIn` `EaseInOut` `Bounce` `AfterPrevious` `WithPrevious` `AtTime` `AtLabel` `Translation` `UTF16Len` `UTF16ByteOffset` `Textf` `DecodeImage` `ImageSize` `ParseSVG` `NewBitmap` `NewCanvas` `Render` `RegisterFont` `FontFamilies` `SafeToOpen` `DataDragOver` `DropData` `NewTextBuffer` `EditableText` `RichText` `TokenField` `ColorPicker` `ColorWell` `DarkTheme` `LightTheme` `Spacer` `MenuButton` `ForwardButton` `Form` `TimeInput` `Tree` `TreeItem` `NewGIFDecoder` `NewVideoPlayer`

**C. base 系列（逻辑层，由框架内部或项目自己的 kit 消费，不直接出现在页面代码里，属正常）**
`CheckboxBase` `CollapsibleBase` `ComboboxBase` `PopoverBase` `SegmentedBase` `SliderBase` `SwitchBase` `TabsBase` `TextInputBase` `TextAreaBase` `ToastBase` `TooltipBase` `ToastViewportBase` `TextInputBuffer(Base)` `TextAreaBuffer(Base)`

> 项目实际是**通过 `ui.Toggle`/`ui.Segmented`/`ui.TextInput` 等高级封装**在用 base，这比自己拼 base 更符合框架意图。

---

## 7. 附录 B：证据索引（文件:行号）

```
go.mod                          mygo v0.7.4 · go 1.27.3 · tool mygo
AGENTS.md:56/57/60/61/73/74/85/89/98    自定纪律（P1-1/P1-5/P2-1/P1-6 的判据）
app.go:34                       store *sqlite.Store
app.go:221                      assets.NewImages(96)
app.go:330-340                  飞行时强制 TransitionNone
state.go:25                     loadFeatureState(ctx, *sqlite.Store)
db.go:50                        func (s *Store) DB() *msqlite.DB
seed.go:9 / :46                 import msqlite / Import(ctx, db *msqlite.DB, …)
m5_nutrition.go:73              var nutritionProfileReader = nutritionReadProfile
m5_nutrition.go:137-166         手写 12 列 SELECT（P0-1）
m5_nutrition.go:458             ui.Meter 正确用法
m5_nutrition.go:594-622         nutritionInt/String/Float
sichu.go:22-26 / :36-70         sichuProfileColumns（19 列）/ GetSichuProfile
pantry_views.go:116 / :566      手写 3 列 / 2 列网格（P1-2）
recipes_views.go:269            ui.GridView 正确用法
feed_views.go:32-116            自建月历（P1-3）
feed_views.go:362/385           ui.Progress 当占比条（P1-4）
m6_profile.go:725               ui.Progress 当排行条（P1-4）
recipe_hero.go:113-117          自绘 4px 轨道（P1-4）
theme.go:22-61                  ApplyTheme + c.SetTheme 每帧（P1-6）
tokens.go:60                    func Sp(c, n) —— 仅 17 处调用
components.go:145-152           design.Chip（ui.Toggle 包装，正确）
shell.go:69-72                  自建侧栏的理由注释
shell.go:73/125                 RoleTree / RoleTreeItem（全项目仅此 2 处 Role）
flight.go:1-3 / gravity.go:1-2  从 Cloudy 复制（AGPL）
assets/images.go:33-35          NewImages(max)
```

---

## 8. 一句话总结

**这是一个真正用 MyGo 写出来的应用，不是套壳**——框架的虚拟化、匹配几何、玻璃材质、图表、事件查询式模型都被吃透了，动效三档纪律执行得比多数项目彻底。

**它的问题不在「没用框架」，而在「用了框架之后，自己又搭了一层没搭完的分层」**：`internal/repository` 写好了却没人调用，于是 `nutritionReadProfile` 被复制出来救急——两个问题是同一个根因。修完 P1-1，P0-1 会自动消失。

**最该现在做的三件事**（按性价比排序）：

1. **批次 1 全部**（一天，零风险，拿回 180 MB 内存 + 修掉 1 个语义 bug）；
2. **`design.Sp` 从 17 处长到 100 处**——不是重写全部，是让 `Spacing` 这个杠杆真正能拨动；
3. **仓储接口零改动替换**（§4 批次 3 的技巧）——只改一个字段类型，20 个调用点不动，测试时间从 278s 掉到十几秒。

**永远别做的**：不要为了「用框架」去换掉 `Flight`、`Tilt`、`Skeleton`、图片 LRU、自建侧栏。框架在这些位置确实有缺口，硬换是退步。
