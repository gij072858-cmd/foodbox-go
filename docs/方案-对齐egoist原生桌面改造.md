# 食匣 · 对齐 egoist 原生桌面改造方案

- **依据 A**：`Gemini审批建议.pdf`（2026-11-06，7 张页面截图对应的批评与组件替换表）
- **依据 B**：`docs/审查-MyGo框架使用审计.md`（同日，对照 MyGo v0.7.4 源码的只读审计）
- **框架基线**：`github.com/egoist/mygo v0.7.4`，源码位于 `$(go env GOMODCACHE)\github.com\egoist\mygo@v0.7.4`
- **本文性质**：只做改造方案，不改代码。每个批次可独立提交、独立验收。
- **命名约定**：`G-xx` = Gemini 建议条目；`A-xx` = 审计报告条目；`SH/PK/SC/RC/RUN/FD/QW/DATA/CL` = 本方案的批次任务。

---

## 0. 一句话诊断

> 食匣的底层是真 MyGo，缺的不是框架，而是**桌面信息架构**：容器套容器、标题叠标题、同一个事实说三遍，以及把「进度 / 占比 / 导航」三种语义混用。

Gemini 与审计报告没有冲突，它们说的是同一件事的两面：

| 视角 | 结论 |
|---|---|
| Gemini（视觉/交互） | 像 Web 端大卡片堆叠，不像 AppKit/Linear 式高密度桌面软件 |
| 审计（工程/语义） | 用了框架，但自建了一部分没搭完的分层；控件语义与无障碍有硬伤 |

所以本方案的第一原则不是「把自绘控件全换成 `ui.*`」，而是：

1. **先减容器，再换控件**（很多松散感来自 padding 与嵌套，不是控件本身）；
2. **一个事实只出现一次**（标题、计数、进度、离线状态）；
3. **语义正确的原生件优先**（`Progress` 表进度，`Meter` 表占比，`Calendar` 表日期）；
4. **只保留 4 个框架真实缺口**：跨页飞行 `Flight`、3D 倾斜 `Tilt`、骨架 `Skeleton`、图片 LRU。

---

## 1. 目标量化（可验收，不靠感觉）

以 **1280×860、浅色、完整动效** 为基准窗口：

| 指标 | 现状 | 目标 |
|---|---|---|
| 顶栏高度 | **78**（标题 20 + 副标题 11 两行）+ 页面内再一行标题 | **44**，二级页只有一行叶子标题 |
| 仓库首屏可见条目 | 固定 3 列 × 174px，约 9–12 个 | **≥ 18 个**（4 列 × 约 128px） |
| 新手包地域入口 | 4×2 巨型卡片，占半屏 | **8 个单行可见**（左列表/单行选择器） |
| 私厨配置区高度 | 约 420px（标题 + 3 段卡片） | **≤ 260px**，生成按钮在首屏 |
| 菜式筛选行 | 2 行 + 横向滚动 19 个胶囊 | **1 行**：4 段分段 + 4 个下拉 + 1 开关 |
| 菜式首屏 | 76px 卡片 2 列 ≈ 12 条（`floor((1012+12)/(360+12))`） | 卡片 **≥ 16 条**（4 列 × 4 行）；表格模式 ≥ 16 行 |
| 掌序「第 X 步」出现次数 | **4 次**（顶栏 / 圆环 / 小蓝字 / 底栏） | **1 次**（顶部 Progress 的数值提示） |
| 食历零记录容器 | 空月历 + 3 张 0 卡 + 空状态卡 = **3 套** | **1 个**空面板 |
| `design.CardPad` 调用 | **29** | **≤ 10**（只留弹层、编辑器分组、结果摘要） |
| `design.StatCard` 调用 | **35** | **≤ 12**（只留营养驾驶舱这类真正需要大数字的页） |
| 手写网格 | 2 处（仓库 3 列 / 目录 2 列） | **0**（`ui.GridView`） |
| 自绘月历 | 1 套（`feedCalendar`） | **0**（`ui.Calendar`） |
| 自绘圆环/圆点 | 2 套（详情页、掌序） | **0**（`ui.Meter` / `ui.Progress`） |
| `ui.Progress` 当占比条 | 3 处 `ui.Progress` + 1 处自绘 4px 条 | **0**（`ui.Meter`） |
| `design.Sp` 调用 | 26 | **≥ 100**（Top3 文件先落地） |
| 全项目 `Role(...)` | 2 处 | **≥ 12**（换组件免费获得） |
| `internal/app` 里 `*sqlite.Store` 出现 | 20 处 | **0**（只在 bootstrap 构造） |
| `internal/app` 测试耗时 | 278s | **≤ 30s**（内存仓储替身） |
| 代码量 | — | 视图层净减 **10–15%**；Gemini 说的 40% 不承诺，原因见 D7 |

---

## 2. Gemini 建议逐条核对（先校正事实，再谈改法）

> 下面这些是「照 Gemini 说的直接改会改错」的地方。审计报告给的是框架源码证据，本方案以源码为准。

| 编号 | Gemini 原话 | 代码/框架事实 | 判定 | 处置 |
|---|---|---|---|---|
| **C1** | 「用输入框加两个圆圈按钮去凑步进器」 | `sichu_views.go:67` 用的就是 **`ui.NumberInput`**；`ui/number.go` 内部自带 − / + 两个按钮与上下键步进 | **不成立** | 保留 `ui.NumberInput`，只把宽度从 140 收到 96，并放进 `ui.Form` |
| **C2** | 「顶部搜索框是手搓的大圆角胶囊」 | `shell.go:59-67` 用的就是 **`ui.SearchField`**（带放大镜、Escape 清空） | **不成立** | 保留，缩高度与内边距；不必重写 |
| **C3** | 「进入子页面靠手动拼 `<` 圆圈按钮返回」 | `subHeader`、菜式详情、粘性页脚用的都是 **`ui.BackButton`** | **大部分不成立** | 问题只是「返回按钮 + 残留父标题」，删父标题即可 |
| **C4** | 「用自定义方格去画日历，直接调用 `ui.Calendar`」 | `ui.Calendar` 的格子只能放日期（`date.go`），**不能显示当天几道菜/菜名首字**；审计 P1-3 已核实这是框架缺口 | **半成立** | 采用混合方案 D3：`ui.Calendar` 做导航，右栏做当日日记；放弃格内密度点 |
| **C5** | 「通过 `ui.SplitView` 左右分栏」 | 框架没有 `SplitView`，正确 API 是 **`ui.Split(c, &w, first, second)`** / `ui.SplitPercent` | **API 名错误** | 方案统一写 `ui.Split` |
| **C6** | 「`ui.Breadcrumbs(c, path, &chosen)`」 | 实际签名是 **`ui.Breadcrumbs(c, items []string, chosen *int)`**；`Router` 也**没有** `Depth()` / `PopTo()`，只有 `Back/Forward/Go/Push/Replace/CanGoBack/CanGoForward/Path` | **API 名错误** | 自己按 `/` 段算深度；面包屑点击用 `router.Replace(父路径)` |
| **C7** | 「`ui.RadioGroup(c, ...).Row()`」 | `RadioGroup` 只是带 `RoleRadioGroup` 的容器，选项是 `ui.Radio(c, &v, value, label)` | **API 形态错误** | `RadioGroup` 容器内嵌 `ui.Row` + 多个 `ui.Radio` |
| **C8** | 「`ui.Stepper` 一行代码自带微调」 | `Stepper(c, *float64, lo, hi, step)` 只有上下箭头，**不显示数值**，且是 float | **不合适** | 人数继续用 `ui.NumberInput` |
| **C9** | 「常备/可选食材用 `ui.TokenField`」 | `TokenField` 是**可编辑**标签，点 × 会删除 token；模板食材是只读内容，删掉会改变模板语义 | **语义不符** | 只读用 `ui.Badge` 流；可选条目用 `ui.Toggle` 勾选 |
| **C10** | 「餐段三胶囊 → `ui.Segmented`」 | `sichu_views.go:54-63` 的餐段是 **多选**（`meals map` 可同时选早/午/晚） | **语义不符** | 餐型/就餐人用 `Segmented`/`RadioGroup`；餐段保留 `ui.Toggle` 多选，只降视觉重量 |
| **C11** | 「`Frameless: true` + Vibrancy」 | 当前已用 `TitleBarHidden`（保留 Windows 顶边缩放）；`Frameless` 在 Windows 会丢掉顶边缩放 | **半成立** | 保留 `TitleBarHidden`；`Transparent + Vibrancy: VibrancySidebar` 作为可选增强，不作为验收条件 |
| **C12** | 「蛋白/脂肪/碳水用 `ui.Meter`，自动 Success/Warning/Danger」 | `MeterLevels` 是**阈值**，不是三种成分各自的颜色；三条 Meter 会变成同一种颜色 | **半成立** | 三条 Meter + 文本标注；接受统一色调，或保留一个 6px 色点做区分 |
| **C13** | 「代码量至少砍 40%」 | 视图层 16,113 行，其中大量是业务文案与状态分支，不是容器代码 | **不现实** | 目标净减 10–15%，删除 3 套自绘系统即可 |
| **C14** | 「4 个原生 `ui.Select` 下拉框」 | 实际签名 **`Select(c, selected *string, options []string)`**，不是变参、也不是 `*int` | **API 形态错误** | 筛选状态用 string 字段，选项传 `[]string`；`Segmented` 才用 `*int` |

### 2.1 Gemini 描述与现状不一致的地方（截图可能来自旧版本）

| Gemini 描述 | 现状 | 结论 |
|---|---|---|
| 菜式「双列臃肿卡片」 | `recipes_views.go:269` 已是 **`ui.GridView` 虚拟化、76px 行高、360 最小宽**；`cols = floor((cw+gap)/(minW+gap))`，1280 下正好是 **2 列** |  Gemini 说的「双列」是准的；真问题是**每卡阴影 + 左色条 + 76px 里塞三行字 + 筛选堆叠**，不是列数 |
| 20 多个胶囊「混在同一行」 | 现状是两行：第一行 4 个库存胶囊 + 排序分段；第二行横向滚动的 13 个 facet | 方向对，数字略有出入 |
| 侧栏「粗大蓝条」 | `shell.go:157` 是 `Match` 驱动的滑动胶囊，选中态 Accent | 不是静态粗条，但**面积与饱和度确实过大** |

**处理原则**：以代码现状为准；Gemini 指出的是「观感方向」，不是逐像素事实。

---

## 3. 决策记录（先定分歧，再开工）

### D1 · 侧栏：保留现有滑动胶囊，不改用 `ui.Sidebar`

- **Gemini**：直接用 `ui.Sidebar` + `SidebarSection` + `SidebarItem`。
- **审计 §5**：`SidebarItem` 外观写死在 `sidebar.go`，无法承载滑动胶囊；自建是「用框架原语做的正确取舍」。
- **决策（已拍板）**：**保留现有滑动胶囊与原侧栏结构，不改用 `ui.Sidebar`，胶囊实现（`navItem` / `navPill` / `ElementTransition`）与动画参数都不动**。
- 只做**不改变胶囊外观**的增量（可选）：徽标、A–Z 首字母跳转、Home/End、分区折叠。这些可以后置，不影响胶囊本身。
- **代价**：仍需自己维护 `RoleTree` / `RoleTreeItem`（已有）。

### D2 · 顶栏：`TitleBarHidden` + 行内 `ui.Toolbar`，不切 `Frameless`

- 保留 `app.go:271` 的 `TitleBarHidden`（Windows 仍可拖边缩放）。
- 顶部 44px 一条：`BackButton` → `ForwardButton` → `Breadcrumbs` → 弹性拖拽区 → 右侧 `ui.Toolbar`。
- 拖拽区只包**空白弹性区**，不包整行，避免吞掉按钮点击。
- `Vibrancy` 只在 Windows 11 有可见效果；先作为可选开关，不写入验收。

### D3 · 食历：`ui.Calendar` 导航 + `ui.Split` 右栏日记（放弃格内密度点）

- 左栏：`ui.Calendar(c, &a.feedDate)` + 一行统计（`12 天 · 38 道 · 连续 5 天`）。
- 右栏：选中日的日记列表 + 最近记录。
- **已确认去掉格内密度点**：`ui.Calendar` 的格子只显示日期，不再画「当天几道菜」的点阵；自绘 `feedCalendar` / `feedCell` 整段删除，不保留 Plan B。
- 补偿：左栏底部一行「本月开饭 N 天」，右栏按日期倒序列出有记录的日子。

### D4 · 菜式列表：卡片流仍是默认，`ui.Table` 作为「紧凑」视图

- Gemini 要「放弃卡片直接 Table」；审计认为 `GridView` 是正确用法。
- 现实约束：跨页飞行 `Flight` 依赖卡片的源矩形，Table 行没有封面。
- **决策（已拍板）**：保留 `GridView` 为默认浏览；新增 `Segmented`「卡片 / 表格」切换，表格模式用 `ui.Table`（可排序、可拖列宽）。表格模式不做飞行，详情直接 `Push`。
- 结果：既满足桌面效率，又不牺牲已有动效资产。

### D5 · 食材标签：只读 `Badge`，可选 `Toggle`，不用 `TokenField`

- 新手包模板是只读数据；`TokenField` 的删除语义会悄悄改模板。
- 若未来允许用户自定义常备清单，再把编辑态单独放进「我的仓库」页。

### D6 · `Meter` 的颜色：接受单色，用文字/色点区分宏量

- `ui.Meter` 只吃主题 Success/Warning/Danger；三条宏量条会同色。
- 折中：每条前保留一个 6px 色点（现有的 `ui.Box`），条本身用 `Meter`；这样既有语义又保留区分。

### D7 · 不承诺「代码 -40%」

- 视图层大头是业务文案、条件分支和状态机，不是容器。
- 可承诺的是：**删除 3 套自绘系统（月历、圆环/圆点、手写网格）+ 收敛 29 处 `CardPad` 到 ≤10**。

---

## 4. Gemini 建议覆盖索引

> 这张表保证 PDF 里每一条都有落点。状态：采纳 / 部分采纳 / 不采纳（附理由）。

| ID | Gemini 条目 | 状态 | 落点 |
|---|---|---|---|
| G-A1 | 滥用白底大卡片 + 胶囊按钮，缺桌面密度 | 采纳 | §5 设计规则；PK-1、SC-2、FD-3 |
| G-A2 | 新手包 8 地域块 / 私厨 4 张空卡 / 食历 30 方块 | 采纳 | PK-3、SC-7、FD-1/2 |
| G-A3 | 手搓单选组、步进器、日历 | 部分采纳 | C1/C4/C7；SC-3、SC-5、FD-2 |
| G-A4 | 离线状态两处重复 | 采纳 | SH-4 |
| G-A5 | 父子标题重复 | 采纳 | SH-1 |
| G-A6 | 私厨双层标题语义重叠 | 采纳 | SC-1 |
| G-A7 | 菜式标题下连写两行「944 道菜」 | 采纳 | RC-1 |
| G-B1 | 侧栏粗蓝条 / 搜索胶囊 / 状态文字 / 手拼返回 | 部分采纳 | D1、C2、C3、SH-1/4 |
| G-B2 | `Frameless` + Vibrancy + `TitleBar` + `Toolbar` | 部分采纳 | D2、SH-2 |
| G-B3 | `Router` + `Back/Forward` + `Breadcrumbs` + 替换主视图 | 采纳（修正 API） | SH-3、C6 |
| G-B4 | `SearchField` + `Sidebar` + `Badge` 挂 15 样/2 道 | 部分采纳 | D1、SH-5 |
| G-C1 | 8 地域改 `Tabs` 或左列表 + 右详情 | 采纳（选列表方案） | PK-3 |
| G-C2 | 常备/可选食材改 `TokenField` | 不采纳（语义） | C9、PK-4 |
| G-C3 | `Form` + `Field` + `Switch` 同行 | 采纳 | PK-5 |
| G-C4 | 底部改 `PrimaryButton` / `Button` | 采纳 | PK-6 |
| G-D1 | 双层标题 / 餐型大卡 / 餐段胶囊 / 人数 / 4 依据卡 | 采纳（餐段除外） | SC-1…SC-7、C10 |
| G-D2 | 删标题，右上角 AI/配额移入 Toolbar | 采纳 | SC-1、SH-2 |
| G-D3 | `Form` 标签右对齐、控件左对齐 | 采纳 | SC-2 |
| G-D4 | 餐型 `RadioGroup` / `Segmented` | 采纳 | SC-3、C7 |
| G-D5 | 餐段 `Segmented` | **不采纳**（多选） | C10 |
| G-D6 | 人数 `NumberInput` / `Stepper` | 采纳（`NumberInput`） | C1/C8、SC-5 |
| G-D7 | 就餐人 `Segmented` | 采纳 | SC-6 |
| G-D8 | 配餐依据一行弱化文本 + `Link` | 采纳 | SC-7 |
| G-E1 | 20+ 胶囊混排 / 红黄绿粗条 | 采纳 | RC-2、RC-3 |
| G-E2 | `Segmented` 库存筛选，数字写进标签 | 采纳 | RC-2 |
| G-E3 | 4 个 `Select` + 当季 `Toggle` | 采纳 | RC-2 |
| G-E4 | 放弃双列卡片，改 `Table` / 紧凑 `List` | 部分采纳 | D4、RC-5 |
| G-E5 | `Table` 表头排序、列宽拖拽、键盘浏览 | 采纳（表格模式） | RC-5 |
| G-E6 | 7 种做法改 `Tabs` / `Select` | 采纳（`Select`） | RC-6 |
| G-E7 | 删圆环/圆点，改 `ui.Meter` + `MeterLevels` | 采纳（单色折中） | RC-7、C12 |
| G-E8 | 食材列表改紧凑 `Table` | 采纳 | RC-8 |
| G-F1 | 掌序信息冗余 / 进度 4 次 / 黑计时条 / 下一步 | 采纳 | RUN-1…RUN-4 |
| G-F2 | 砍上方大卡与圆环，只留步骤流 | 采纳 | RUN-1 |
| G-F3 | 顶部细 `Progress` + 总耗时 | 采纳 | RUN-2 |
| G-F4 | 移除下一步与悬浮黑框，倒计时内联当前行 | 采纳 | RUN-3/4 |
| G-G1 | 30 个空白日期块，首行被裁切/阴影 | 采纳 | FD-2、D3 |
| G-G2 | 零记录时三重空态 | 采纳 | FD-1 |
| G-G3 | 参考 godiff `emptyPanel` + `PrimaryButton` | 采纳 | FD-1 |
| G-G4 | `Calendar` + 左右分栏 | 部分采纳（修正 API、已确认去掉密度点） | D3、FD-2、C4/C5 |
| G-H1 | 8 行组件替换速查表 | 采纳（修正版见 §7） | 全表 |
| G-H2 | 删自绘控件，代码 -40% | 部分采纳 | D7 |

### 4.1 审计报告条目覆盖

| ID | 审计条目 | 本方案批次 |
|---|---|---|
| A-P0-1 | `nutritionReadProfile` 与 `GetSichuProfile` 分叉 | DATA-2 |
| A-P1-1 | 仓储层死代码，页面直连 SQLite | DATA-1 |
| A-P1-2 | 手写网格替代 `ui.GridView` | PK-1 |
| A-P1-3 | 自建月历 | FD-2 / D3 |
| A-P1-4 | `ui.Progress` 当占比条 | QW-1 |
| A-P1-5 | `seed` 绕过事务、`Store.DB()` 漏连接 | DATA-3 |
| A-P1-6 | `ApplyTheme` 每帧 `SetTheme`、`ui.Themed` 文档谎言 | QW-5、CL-3 |
| A-P1-7 | `ReleasePixels` 零调用 | QW-3 |
| A-P1-8 | AGPL 复制单元的升级成本 | CL-4 |
| A-P2-1 | 设计令牌被系统性绕过 | CL-1 |
| A-P2-2 | `NewImages(96)` 低于文档 | QW-4 |
| A-P2-3 | `design` 7 个导出无人使用 | QW-2 |
| A-P2-4 | 手写列表/面板缺 `Role` | CL-2 |
| A-P2-5 | mpv 缺失时静默失败 | CL-5 |
| A-§5 | 不要换掉 Flight/Tilt/Skeleton/LRU/侧栏/自定义 Modal/mpv | 全方案遵守 |

---

## 5. 设计规则（先立规矩，再改页面）

这 12 条是后续所有改动的判据，也是 Code Review 的 checklist。新代码违反即打回。

### 5.1 容器

1. **卡片只给「可点击对象」和「弹层」用**。页面区块用「标题 + 分隔线 + 背景分组」，不再用 `design.CardPad`。唯一例外是**空状态**（整个页面只有它一个面板）。
2. **禁止卡片套卡片**。一个白色圆角面里不能再出现第二个白色圆角面；内层用背景差或分隔线。
3. 列表行一律无独立阴影、无独立圆角外框；用 1px 分隔线或 hover wash。
4. 页面左右边距统一 `design.PagePadding`；区块间距 24，区块内行距 8/12，禁止 32/36 这类大 padding 当默认。

### 5.2 形状与字号

5. 普通按钮 **不再用 `Radius(999)`**；胶囊只用于 `Badge`/`Pill`/`Tag`。允许的圆角只有 8（行/输入）、12（卡片/弹层）、999（徽标）。
6. 字号只保留三档：**20 标题 / 13 正文 / 11 辅助**。删除页面里 15/16/17/18 的混用。
7. 一个屏幕最多 **1 个主要按钮**；其余用次级或文字按钮。

### 5.3 标题与计数

8. **一屏一标题**：二级页顶栏只显示叶子标题；页面正文不再重复同一标题。
9. 计数只出现一次：要么在筛选标签里（`能做(17)`），要么在页面副标题里，不能两处都写。
10. 主标题下最多一行副标题，且只在根页面出现。

### 5.4 语义

11. `Progress` 只表「进行中的进度」（当前步/总步、上传/启动）；`Meter` 表「静态占比/水位」（库存覆盖、宏量构成、排行）。
12. 状态只在需要用户行动的地方出现。本地离线是默认事实，**不在壳层常驻提示**；只有 AI/网络相关页面才提示「本地策略」。

---

## 6. 批次计划

> 顺序按依赖排：先门禁 → 零风险 → 壳层 → 数据 → 页面。页面批次之间互不依赖，可并行。

### 阶段 0 · 基线

**目的**：改之前先有可对比的证据，避免「感觉更松/更紧」。

- 在 `test/screenshots/before/` 保存 7 张页面截图：仓库、新手包、私厨、菜式列表、菜式详情、掌序执行、食历（1280×860 浅色；1024×720 浅色；1280×860 深色）。
- 记录基线计数（PowerShell）：

```powershell
cd "<repo>"
foreach ($p in @('design\.CardPad','design\.StatCard','design\.Chip\(','ui\.Progress','ui\.Meter','ui\.GridView','design\.Sp\(','\.Role\(','Radius\(999')) {
  $n = 0
  Get-ChildItem internal\app\*.go,internal\design\*.go | ForEach-Object {
    $n += (Select-String -Path $_.FullName -Pattern $p -AllMatches).Matches.Count
  }
  "$p = $n"
}
```

- 四项门禁基线：`go build ./...`、`go vet ./...`、`go tool mygo vet .`、`go test ./... -count=1`。
- **产出**：`test/screenshots/before/` + 一份计数记录（放提交说明里即可，不额外建文档）。

**验收**：四项全绿；截图齐全；计数与本文 §1 的「现状」列一致（允许 ±3）。

---

### 阶段 1 · 零风险清理（QW）

对应审计批次 1。不碰业务逻辑。

| 任务 | 改动 | 文件 | 验收 |
|---|---|---|---|
| **QW-1** | 3 处 `ui.Progress` + 1 处自绘条 → `ui.Meter` | `feed_views.go:362/385`、`m6_profile.go:725`、`recipe_hero.go:113-117` | 读屏语义为 level indicator；截图仅条色变化 |
| **QW-2** | 删除 7 个死导出；`OverlayTint`/`ModalPane` 改小写 | `internal/design` | `go build`；`go test ./internal/design` |
| **QW-3** | `ReleasePixels` 接入图片 LRU | `internal/assets/images.go` | 连续切换仓库/菜式 5 次，RSS 下降；`go test ./internal/assets` |
| **QW-4** | `assets.NewImages(96)` → `160` | `app.go:221` | 与方案文档 128–256 对齐 |
| **QW-5** | 修正 AGENTS.md 的 `ui.Themed` 描述；补一条「`ApplyTheme` 每帧投影」 | `AGENTS.md:89/98` | 文档与代码一致 |

**为什么先做**：它把「语义 bug」和「180MB 常驻内存」先拿掉，后面改页面时截图差异更容易归因。

**回滚**：单独一个 commit；QW-1 可逐文件回退。

---

### 阶段 2 · 壳层重构（SH）

对应 G-B1…G-B4、G-A4/A5，以及审计 P1-6 的文档部分。

#### SH-1 顶栏：44px，一屏一标题

**现状**：`shell.go:164-184` 的 `topBar` 无条件渲染 `pageTitle()` 的父级标题与副标题；`/pantry/starter` 又在 `pantry_views.go:636` 画 `subHeader("新手包")`；`/sichu` 更在 `sichu_views.go:22` 再画一次「今天怎么吃」。

**改法**：

1. 新增 `routeLeafTitle(path) (title string, sub string)`：只返回叶子段标题，副标题仅根页面返回。
2. `topBar` 结构改为：

```go
ui.Row(c).FillWidth().Height(44).Padding(12, 0).AlignItems(ui.Center).Children(func() {
    if routeDepth(a.router.Path()) > 1 {
        ui.BackButton(c, a.router)
        ui.ForwardButton(c, a.router)
        crumbs := a.breadcrumbItems()          // []string
        ui.Breadcrumbs(c, crumbs, &a.crumbChosen) // chosen 回调 router.Replace(父路径)
    } else {
        ui.Text(c, leafTitle).FontSize(design.FontHeading).FontWeight(600)
    }
    ui.Row(c).Grow(1).Height(44).DragWindow()  // 只让空白区拖窗
    ui.Toolbar(c, func() { a.topActions(c, p) })
})
```

3. 删除 `topBar` 里父标题 + 副标题的组合；根页面的副标题下沉到页面首行（`pageBody`），不在顶栏重复。
4. 二级页的 `subHeader` 保留，但只画「叶子标题 + 计数/动作」，不再画返回按钮（返回已在顶栏）。

**关键约束**：`ui.Breadcrumbs` 的 `chosen` 是 `*int`；点击后调用 `a.router.Replace(crumbs[chosen])`（不要用 `Back()`，历史栈会错位）。`routeDepth(path)` 按 `/` 段计数即可，框架没有现成的深度 API。

**验收**：
- `/pantry` 顶栏：`仓库` + 44px；`/pantry/starter` 顶栏：`仓库 › 新手包`，正文只有一次「新手包」。
- `/sichu` 全屏只出现一次「私厨」相关标题。
- 1024×720 下顶栏不折行，返回/前进可点。

#### SH-2 右侧动作区用 `ui.Toolbar`

**改法**：把「AI 与配额」「新建菜式」「设置」等动作统一放进 `ui.Toolbar`，自动获得工具栏尺寸、键盘焦点与溢出菜单（`»`）。注意 `Toolbar` 只对**按钮/开关/分段/菜单按钮**做溢出收集；不要把 `Breadcrumbs` 或 `SearchField` 塞进去（否则窄窗不会折叠）。

- 私厨的「AI 已配置 · 剩 N 次」从页面右上角移到 Toolbar 的一个状态按钮，点击进 `/settings/ai`。

**验收**：1024 宽时右侧动作收进溢出菜单，不再横向溢出。

#### SH-3 前进/后退与面包屑

**现状**：只有 `ui.BackButton`，没有前进；面包屑只在菜式详情用了一次。

**改法**：
- 顶栏统一 `BackButton` + `ForwardButton`（`ui.Router` 本身支持历史栈）。
- `breadcrumbItems()` 由 `routes.go` 的静态标题表生成（`仓库 / 新手包`），**不要**从路径字符串 split 后直接展示（`starter` 会显示成英文）。
- `chosen` 回调只允许跳到已存在的父路径。

**测试**（`ui.NewTester`）：
- Push `/pantry/starter` → 点返回 → path 回 `/pantry`；
- 再点前进 → 回 `/pantry/starter`；
- 点面包屑「仓库」→ 直接回 `/pantry`。

#### SH-4 删除离线文案

**现状**：`shell.go:93` 侧栏底部「离线单机 · 数据只在本机」；`shell.go:181` 顶栏右侧「● 离线 · 数据在本机」；`shell.go:35` 的根页面副标题还是「离线单机的家庭厨房」。

**改法**：三处都删/改。离线是本地应用的默认事实；把「数据只在本机」移到 `设置 > 关于` 的一行说明。根页面副标题改成功能描述（「按现有食材找今天能做的菜」），不写部署形态。只有 AI/视频这类**需要联网**的功能，才在对应页面提示当前是「本地策略」。

**验收**：`Select-String -Pattern "离线"` 在 `internal/app` 只剩设置/关于与 AI 页面各 1 处。

#### SH-5 侧栏增量（可选；胶囊保持原样）

> 已拍板：滑动胶囊、`navItem`、`navPill`、`ElementTransition` 与动画参数**都不动**。以下只是可选增量，可以后置，改动不得影响胶囊外观与滑动路径。

**徽标**：
- `navItem` 增加 `badge string` 字段；渲染在标题右侧，用 `ui.Badge`，字号 11，背景 `p.AccentPale`，文字 `p.Accent`。
- 仓库：`len(a.pantryRows)`（示例 15）；掌序：`len(a.sched.prep)` 或待做步数（示例 2）；为 0 不渲染。
- 不要把计数写进 `Label`（读屏会念成标题的一部分）。

**键盘**：
- 现有 `FocusGroup(ui.Vertical)` 已给上下键；补：
  - `Home` / `End`：跳首/末项；
  - A–Z：在 nav 容器注册 26 个 `c.OnShortcut(0, ui.KeyA+i, ...)`，维护 300ms 的 `navTypeahead`，匹配 `label` 前缀（中文用拼音首字母表，或直接匹配中文首个字）。
- 分区折叠：用 `ui.Collapsible` 包「厨房 / 记录 / 我的」，折叠状态存 `prefs`。

**验收**：胶囊位置/滑动时长与改动前一致；按 `c` 跳到「菜式」；`End` 到「设置」；折叠后状态重启仍在。

#### SH-6 主题文档收口（与 QW-5 合并）

- `AGENTS.md` 的 `ui.Themed` 两条改为：「Accent 由 `design.ApplyTheme` 每帧投影到 `ui.Theme`」。
- `ApplyTheme` 增加 `themeSig{Dark, Name}` 比较，只在变化时 `c.SetTheme`；其余帧直接返回缓存指针（行为不变，少一次整窗主题写入）。

**风险**：`SetTheme` 变化时浮层会重建，注意在弹层打开时切主题的测试。

---

### 阶段 3 · 数据分层收口（DATA）

对应 A-P0-1、A-P1-1、A-P1-5。与页面批次可并行，但必须在 RC-5 之前完成（菜式表格要用统一 Reader）。

#### DATA-1 仓储接口零改动替换

- 把 `internal/repository/interfaces.go` 的 7 个接口合并为 3 个：`Reader` / `Writer` / `Admin`，只保留 `App` 真正调用的方法。
- `App.store` 字段类型 `*sqlite.Store` → `Repository`；`loadFeatureState` 参数同理。
- 20 个 `store.Xxx()` 调用点**一行不改**（方法集是 `*sqlite.Store` 的子集）。

**验收**：`Select-String -Path internal\app\*.go -Pattern "sqlite\.Store"` = 0；`bootstrap.go` 只构造一次。

#### DATA-2 删除 `nutritionReadProfile`

- 删 `m5_nutrition.go:137-166` 与 `nutritionInt/nutritionString/nutritionFloat`（594-622）。
- `GetSichuProfile` 改成 `(domain.SichuProfile, bool, error)`；`hasProfile` 由 `ok` 判断。
- 测试缝改为注入 `profileReader func(ctx) (p, ok, err)` 的内存实现，不再复制 SQL。

**验收**：营养页、私厨页、成员卡读同一份 19 列数据；新增测试断言 `allergens/restrictions` 能读到。

#### DATA-3 `seed` 收口

- `seed.Import(ctx, store *sqlite.Store, progress)`，内部改用 `store.Run([]Statement)`。
- `Store.DB()` 若只剩测试用，改 `DBForTest()`；能删则删。
- `internal/app` 中不得出现 `plugins/sqlite`。

#### DATA-4 测试替身

- 新增 `internal/repository/memory`（仅测试用），实现 `Reader/Writer/Admin`。
- `internal/app` 的 `newTestStore(t)` 改注入内存实现；保留 3–5 个真库集成测试（迁移、备份、seed）。

**验收**：`go test ./internal/app -count=1` ≤ 30s；真库集成测试仍覆盖十表备份 round-trip。

---

### 阶段 4 · 仓库与新手包（PK）

对应 G-A1、G-C1…G-C4、A-P1-2。

#### PK-1 两处手写网格 → `ui.GridView`

- `pantry_views.go:116`（3 列）、`:566`（目录 2 列）。
- 参数：仓库 `minWidth 240, height 128`；目录 `minWidth 320, height 96`。`GridView` 的列数公式是 `floor((cw+gap)/(minW+gap))`，1280 下仓库正好 4 列。
- 卡片函数保留，但去掉 `RestShadow` 与 `RadiusCard` 的「白卡」层：改成 `RadiusSmall(8)` + 1px `p.Border` + hover wash；数量与保质期用 11px 辅助字。

**验收**：1024 宽自动降为 3 列；重排后滚动位置保持（GridView 自带）；文档页脚不再横向溢出。

#### PK-2 仓库卡片密度

- 174px → 128px：标题 13/600，第二行「数量 · 位置」，第三行只留保质期状态（`Pill`）。
- 操作按钮（加减/编辑）悬浮时才出现，避免每卡常驻 3 个按钮。
- 顶部库存筛选（`pantryFilterChip`，`:148-159`）是**单选**，从 `design.ChipTone` 改为 `design.Segmented`：`全部 / 在用 / 临期 / 用完`，数字进标签（`在用(15)`）。

**验收**：1280×860 首屏 ≥ 18 个条目；库存筛选只有一行。

#### PK-3 新手包：8 地域改左列表 + 右详情

- 删 `ui.Grid(c).Columns(4)`（`pantry_views.go:647`）与 4×2 大卡。
- `ui.Split(c, &a.packSplit, left, right)`：左栏 220px `ui.List`（8 行、40px、单选），右栏详情（常备/可选 + 说明 + 底部按钮）。
- 或退化为单行 `ui.Select(c, &a.packSel, []string{"广东潮汕", "川湘", …})`，下方详情。二选一，默认 Split。

**验收**：8 个地域同屏可见；切换地域右栏即时更新；无巨型卡片。

#### PK-4 食材标签：只读 Badge + 可选 Toggle（D5、C9）

- 常备：`ui.Badge` 流（`ui.Row().Wrap()`），无 ×、不可删。
- 可选：每项 `ui.Toggle(c, &on, name)`；已选显示 `p.AccentPale` 底，未选透明 + 1px 边框。
- 删除 `design.Pill` 的灰底大胶囊用法（保留 `Badge` 的紧凑尺寸）。

**验收**：点击可选标签切换状态；键盘 Tab 可逐个到达；没有「点 × 删除模板食材」的路径。

#### PK-5 开关与说明同行

- 删 `pantry_views.go:672-689` 的「Switch 一行 + 说明一行」。
- 改为：

```go
ui.Form(c, func() {
    ui.Field(c, "同时加入可选条目", func() {
        ui.Switch(c, &a.packIncludeOptional)
    }).Description("默认只加常备底料；打开后连可选条目一起入库。")
})
```

- `ui.Field` 负责标签与控件的对齐，`.Description` 负责下方 11px 辅助文本；不要手写第二行 `ui.Text`。
- 框架文档明确：点标签会聚焦控件、或直接切换 `Switch`/`Radio`，所以不需要额外绑点击事件。

**验收**：点击标签文字也能切换（Field 的标签激活行为）；1024 宽不换行。

#### PK-6 底部按钮

- `ui.PrimaryButton(c, "加入这些食材")` + `ui.Button(c, "回到仓库")`，左对齐，间距 8。
- 删除 `Radius(999)` 大胶囊与整宽拉伸。

**验收**：一个主按钮；按钮高度 28/32，与顶栏 Toolbar 按钮同档。

---

### 阶段 5 · 私厨（SC）

对应 G-A6、G-D1…G-D8、C1/C7/C8/C10。

#### SC-1 删页面标题，动作上移

- 删 `sichu_views.go:20-37` 的「今天怎么吃」+ 副标题 + 右上 AI 区。
- 顶栏 SH-2 的 Toolbar 放「AI 与配额」；页面直接从 `ui.Form` 开始。
- 结果页（`/sichu/result`）保留自己的标题，因为它是独立内容页。

**验收**：私厨首页全屏只出现一次「私厨」；首屏能看到生成按钮。

#### SC-2 配置区 = 一个 `ui.Form`

```go
ui.Form(c, func() {
    ui.Field(c, "餐型", func() { /* SC-3 */ })
    ui.Field(c, "餐段", func() { /* SC-4 */ })
    ui.Field(c, "人数", func() { ui.NumberInput(c, &a.sichu.servings, 1, 12, 1).Width(96) })
    ui.Field(c, "谁吃这一餐", func() { /* SC-6 */ })
})
```

- 删 `sichu_views.go:49-50` 的 `Padding(16).Radius(14).Background(p.Surface).Border(...)` 容器。
- 删 `a.section` 的三段标题（餐型/餐段与人数/配餐依据），Field 标签就是标题。

**验收**：配置区高度 ≤ 260px；无嵌套卡片。

#### SC-3 餐型：`RadioGroup` + `Row`（C7）

```go
ui.RadioGroup(c, func() {
    ui.Row(c).Gap(16).Children(func() {
        ui.Radio(c, &a.sichu.style, 0, "两菜一汤")
        ui.Radio(c, &a.sichu.style, 1, "一碗端")
    })
})
```

- 说明文字（「一荤一素一汤」）放到 Field 下方的一行 11px 辅助文本，不再塞进卡片。

#### SC-4 餐段：保留多选，降重量（C10）

- 仍是 3 个 `ui.Toggle`（早/午/晚），但去掉 `design.Chip` 的胶囊底：未选透明 + 下划线 hover，选中 `p.AccentPale`。
- 不换 `ui.Segmented`：餐段是多选，Segmented 是单选语义。

#### SC-5 人数：保留 `ui.NumberInput`（C1/C8）

- 宽度 96；`ui.Form` 自动右对齐标签。
- 不换 `Stepper`（无数字显示）。

#### SC-6 就餐人：`Segmented` + 成员管理链接

- ≤4 人：`design.Segmented(c, &a.sichu.whoIdx, "本人", "妈妈", …)`。
- >4 人：`ui.Select(c, &a.sichu.whoName, names)`；`whoName` 为唯一状态源，`whoIdx` 由它派生。
- 「管理成员」从胶囊按钮改为 `ui.Link(c, "管理成员", "/sichu/members")`，放在 Field 右侧。

**验收**：切换成员后生成按钮的份数文案同步变化。

#### SC-7 配餐依据：一行弱化文本 + Link

- 删 `sichu_views.go:95-128` 的 4 个 `StatCard` 与白卡容器。
- 改为：

```go
ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
    ui.Text(c, a.sichuBasisLine()).FontSize(11).TextColor(p.Faint) // 「配餐依据：未填写身体档案」
    ui.Link(c, "去完善", "/sichu/body")
})
```

- 已有的忌口 `Pill` 流保留，但改成 `ui.Badge` 并限制一行（超出显示「+3」）。

**验收**：4 个「—」消失；无「空卡片」；Link 可键盘到达。

#### SC-8 底部动作

- `ui.PrimaryButton(c, "生成配餐")` + 条件性 `ui.Button(c, "看上次方案")`；加载态 `loading.Arc` 同行。
- 删「还没填身体档案也能配…」这句（已由 SC-7 的依据行表达）。

---

### 阶段 6 · 菜式列表与详情（RC）

对应 G-A7、G-E1…G-E8、A-P1-4 的收尾。

#### RC-1 删除重复计数

- 删 `recipes_views.go:211-217` 的「N 道菜式 / 现在能做 …」两行。
- 数字进 `Segmented` 标签：`全部`、`能做(17)`、`差一点(230)`、`差太多(697)`。
- 「新建菜式」移到顶栏 Toolbar（SH-2）。

#### RC-2 单行筛选栏（G-E2/E3）

```go
ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
    design.Segmented(c, &a.recipeEdit.stock, "全部", stockLabel("能做", a.canCook), ...)
    ui.Row(c).Grow(1) // spacer
    ui.Select(c, &a.recipeEdit.mainFilter, []string{"主料不限", "猪肉", "鱼", ...})
    ui.Select(c, &a.recipeEdit.regionFilter, []string{"菜系不限", "川菜", "粤菜", ...})
    ui.Select(c, &a.recipeEdit.methodFilter, []string{"做法不限", "快炒", "蒸", ...})
    ui.Select(c, &a.recipeEdit.sortFilter, []string{"默认", "最快", "最新", "热量"})
    ui.Toggle(c, &a.recipeEdit.seasonOnly, "当季")
})
```

- 删 `ui.ScrollHorizontal` 的 facet 行（`:244-248`）与 `design.ChipTone`（`:804/:818`）。
- 删除「本月 N 月」小字（放进「当季」Tooltip）。
- `recomputeRecipeView` 改为读 5 个筛选字段（stock + 4 个 string filter）；`recipeFacets` 由静态表驱动下拉选项。

**验收**：1100 宽一行放下；1024 宽时把「菜系 / 做法」两个低频下拉收进一个「筛选」`ui.PopoverMenuButton`，其余保留；无横向滚动。

#### RC-3 去掉左色条

- 删 `recipeCard` 左侧 4px 的 `recipeMarkBar` 红/黄/绿粗条（`recipes_views.go:290-330` 附近）。
- 状态改为行内一个小圆点 + 文本「能做 / 差 2 样 / 差太多」，颜色只用 `p.Success/Warning/Faint`。

**验收**：删除后卡片左缘整齐；状态仍可扫读。

#### RC-4 卡片密度

- `GridView` 高度 76 → 64；`minWidth 360` → `240`；1280 下 4 列（4×240+3×12=996 ≤ 1012）。
- 删每卡的 `RestShadow`；用 1px 边框；hover 只做背景 wash。
- 删 `recipes_views.go:581` 的 24×24 装饰底圈（`Radius(999)` + `AccentPale`），需要强调时只用文字。
- 封面 40×40（原 56），标题 13/600，副行 11（菜系 · 耗时 · 热量）。

**验收**：1280×860 首屏 ≥ 16 条卡片。

#### RC-5 表格模式（D4）

- 筛选栏右侧加 `Segmented`「卡片 / 表格」。
- 表格：

```go
cols := []ui.TableColumn{
    {Title: "菜式", Sortable: true, MinWidth: 180},
    {Title: "菜系 · 分类", Sortable: true, Width: 160},
    {Title: "耗时", Sortable: true, Width: 80, Align: ui.End},
    {Title: "热量", Sortable: true, Width: 80, Align: ui.End},
    {Title: "库存", Width: 110},
}
ui.Table(c, &a.recipeTable, cols, len(a.recipeView), func(row, col int) { ... })
```

- 排序：`a.recipeTable.Sort` 变化时重算 `recipeView`（先排序再喂 Table，Table 不自己排数据）。
- 库存列：`ui.Badge`「能做 / 差 N 样」。
- 表格模式不做飞行；`a.flightFrom` 清空，`router.Push` 直接过渡。

**验收**：点表头「耗时」按升/降排序；拖列宽后重启保留（`ListState.Columns`）；键盘上下浏览 944 条不卡顿。

#### RC-6 做法切换：`ui.Select`

- 详情页 `recipes_views.go` 里两排做法胶囊 → 单行 `ui.Select(c, &a.recipeDetail.method, labels)`（`labels []string`） + 下方 11px 说明。
- 7 个中文标签用 `Tabs` 会挤，不用 Tabs。

#### RC-7 圆环与圆点 → `Meter`（C12）

- 删 `recipe_hero.go` 的 `design.Ring` 调用、4px 自绘宏量条、`••••••○○○○` 圆点串。
- 宏量：

```go
// 宏量构成：value = 该宏量占一餐能量的比例，hi = 1；三条同色，靠 6px 色点 + 文字区分
ui.Meter(c, float64(share(prot, total)), 0, 1, nil).Label("蛋白 32%")
```

- 必需食材覆盖：

```go
ui.Meter(c, float64(have), 0, float64(need),
    &ui.MeterLevels{Critical: 0.34, Warning: 0.99}).Label("必需食材在库")
```

- 宏量条是「构成占比」，`levels` 传 `nil`；覆盖条是「水位」，才用 `MeterLevels`：100% 才 Success，低于 100% Warning，低于 50% Critical。`MeterLevels` 的低向阈值语义在 `ui/indicators.go` 有说明。
- **删除 `design.Ring`**（掌序同步改），`recipe_hero.go` 只保留标题/动作/统计文本。

**验收**：详情页无 SVG 圆环与圆点；读屏念「必需食材在库，3/6，水位」；浅/深色下 Meter 轨道对比度 ≥ 3:1。

#### RC-8 食材表

- 底部食材列表改 `ui.Table`：列「食材 / 用量 / 在库」。行高 30，`ui.Badge` 表示有/缺。
- 10–30 行不需要虚拟化，但 Table 免费给 `RoleTable` 与键盘逐格浏览。
- 调料与主料用一张表，靠「类型」列区分（或分组行），不再用三种 Tag 堆在行尾。

**验收**：一屏放下 8–10 行；缺料行有明确 Badge；Tab 可进入表格。

---

### 阶段 7 · 掌序执行（RUN）

对应 G-F1…G-F4。

#### RUN-1 砍掉上半部大卡与圆环

- 删 `schedule_views.go:542-584` 的 `runHero`（`design.CardPad` + `design.Ring` + 「下一句」重复展示）。
- 歌词流本身已展示当前行，hero 是第二遍。

#### RUN-2 顶部只留细 `Progress` + 总耗时

```go
ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
    ui.Progress(c, float64(a.sched.runStep+1)/float64(len(a.sched.runLines))).Height(2).Grow(1)
    ui.Text(c, fmt.Sprintf("%d / %d · 约 %d 分钟", a.sched.runStep+1, total, minutes)).
        FontSize(11).TextColor(p.Faint)
})
```

- 进度数值只在这里出现一次。删 `schedule_views.go:434` 的「N 步 · 已用」、`:563` 的「第 X 步」、`:720` 的「第 X / Y 步」。
- 保持 `Progress` 语义：这是进行中的进度，不是占比（与 A-P1-4 不冲突）。

#### RUN-3 倒计时内联当前行

- 删 `schedule_views.go:596-624` 的绝对定位黑色胶囊（`Radius(999).Background(p.Text)`）。
- 把倒计时、挂起、完成三个控件放进**当前激活行**的右侧：

```go
ui.Row(c).Gap(6).Children(func() {
    ui.Text(c, a.sched.runTimerLabel()).FontSize(11).TextColor(p.Muted)
    if design.Button(c, p, "run-hang", "挂起", design.ButtonGhost).Clicked() { ... }
})
```

- 当前行是歌词流里唯一带背景 wash 的行，控件随它移动，不会遮挡其它行的时钟按钮。

#### RUN-4 删除底部「下一步」

- 删 `:710-726` 的底栏「下一步」按钮与说明文字。
- 前进仍可用：点击下一行、左右方向键、`Space` 完成当前步。把这些写进当前行的 `ui.Tooltip`，不占版面。
- 暂停/继续保留为顶栏 Toolbar 的文字按钮（`ui.Button` / `design.Button` 幽灵态）。框架**没有** `ui.IconButton`，不要照 Gemini 的写法去找。

**验收**：
- 全屏「第 X 步」类文字只有顶部一处；
- 1024×720 下逐个点击三行的时钟按钮都命中（无覆盖）；
- 步骤推进后 `Progress` 比例变化，最后一行为 1.0；
- `tt.FramePending()` 在静止时仍为 false（动效结束停帧）。

---

### 阶段 8 · 食历（FD）

对应 G-G1…G-G4、A-P1-3。

#### FD-1 零记录：一个空面板

```go
if len(a.feed.records) == 0 {
    a.pageBody(c, func() {
        design.EmptyState(c, p, &icons.Feed, "还没有开饭记录",
            "去掌序做一道菜，回来这里就有第一天。").
            Children(func() {
                if ui.PrimaryButton(c, "去掌序做一道菜").Clicked() {
                    a.router.Push("/schedule")
                }
            })
    })
    return
}
```

- 在 `feedPage` 入口提前返回；不再渲染 `feedCalendar` / `feedStats` / 底部空卡。
- 参考 godiff 的 `emptyPanel`：居中、最大宽 420、只有一个主按钮。

**验收**：0 记录时页面上只有 1 个空面板、1 个主按钮；不再叠月历与统计。空面板允许保留（唯一例外），但去掉阴影与内层卡片。

#### FD-2 有记录：`ui.Split` 左右分栏（D3、C4/C5）

```go
ui.Split(c, &a.feedSplit, func() {
    ui.Column(c).Gap(12).Padding(16).Children(func() {
        ui.Calendar(c, &a.feedDate)
        ui.Text(c, a.feedMonthSummary()).FontSize(11).TextColor(p.Faint)
        // 「本月开饭 12 天 · 连续 5 天」
    })
}, func() {
    // 选中日的日记列表 + 最近记录
    a.feedDayList(c, p)
})
```

- 删 `feed_views.go:32-79` 的自建月历与 `:83-116` 的 `feedCell`，删「上月/下月/本月」按钮（Calendar 自带）。
- `ui.Calendar` 选中日期写回 `a.feedDate`，右栏按该日重建（在 `Changed()` 时重算）。
- **已确认去掉格内密度点**；`feedCalendar` 与 `feedCell` 整段删除，不保留自绘格。

#### FD-3 统计不占卡

- 删 `feed_views.go:119-142` 的 4 个 `StatCard`（review 页的统计另算）。
- 变成左栏 Calendar 下方一行：`12 天 · 38 道 · 连续 5 天`，11px `p.Muted`。

#### FD-4 月历可访问性

- `ui.Calendar` 自带 `RoleTable` / `RoleRow` / `RoleCell` / `RoleButton`、方向键、PageUp/PageDown、本地化星期头；不再补 Role。
- 若未来重新要回密度点，必须补 `RoleTable` + `RoleCell` + `.Label`，并把格子降到 28px、无白卡、无阴影；当前不做。

**验收**：
- 1 条记录时，1024×720 首屏同时看到月历与日记；
- 点日期右栏切换；
- 键盘 `←/→/↑/↓/PageUp/PageDown` 选日（Calendar 内置）。

---

### 阶段 9 · 收口（CL）

| 任务 | 内容 | 验收 |
|---|---|---|
| **CL-1** | 令牌收敛 Top3：`m5_sichu.go`、`schedule_views.go`、`pantry_views.go` 的硬编码 padding/gap/width 改 `design.Sp(c, n)`；新增代码强制 `Sp` | `design.Sp` ≥ 100；`Spacing` 从 4 调到 3/5 时三页整体收紧/放松 |
| **CL-2** | 无障碍：换组件免费获得 Role；图标按钮补 `.Label`；自建 nav 保留 `RoleTree/RoleTreeItem` | `Role(` ≥ 12；键盘全流程可完成一次配餐 |
| **CL-3** | `ApplyTheme` 签名缓存；`ui.Themed` 只在设置页主题预览使用 | 切主题时整窗更新；普通帧不写主题 |
| **CL-4** | `AGENTS.md` 补 AGPL 条款：`flight.go`/`gravity.go` 保留头注释行号，上游变更需同步 | 文档检查 |
| **CL-5** | mpv 缺失时给明确文案（设置页「视频预览需要本机 mpv」），不黑屏 | 未安装 mpv 的机器点视频 → 有提示 + 回退 WebView |
| **CL-6** | 删除本方案产生的死代码：`design.Ring`、`feedCalendar`、`feedCell`、`recipeMarkBar`、`design.ChipTone`（若 PK-2/RC-2 后无调用） | `go build` + `go vet` + `mygo vet` + grep 计数 = 0 |
| **CL-7** | 全项目 `design.Chip` 语义梳理：**单选组**换 `Segmented`/`Select`（`m6_profile.go:592-626` 的时间/主料/菜系筛选），**多选**保留 `ui.Toggle`（月份、厨具、忌口） | 单选 Chip = 0；多选仍可键盘逐个切换 |
| **CL-8** | 去卡片化抽查：`settings_views.go`、`profile_views.go`、`m6_profile.go`、`m5_nutrition.go`、`m3_recipe_editor.go` 里**非可点击区块**的 `CardPad` 改为 `SectionHeader` + `ui.Divider`；只保留弹层、编辑器分组、结果摘要 | `design.CardPad` 总数 ≤ 10；`StatCard` ≤ 12 |

---

## 7. 组件替换速查表（修正版）

| 页面模块 | 现状 | 目标 API | 备注 |
|---|---|---|---|
| 窗口与顶栏 | `TitleBarHidden` + 自绘 56px 顶栏 | `TitleBarHidden` + 44px 行 + `ui.Toolbar` | 不切 `Frameless`（Windows 缩放） |
| 侧边导航 | 自建 `ButtonBase` + `Match` 胶囊 | 保留 + `ui.Badge` + 首字母/Home/End | 不换 `ui.Sidebar`（D1） |
| 全局搜索 | `ui.SearchField`（已在用） | 保留，收高度 | 不是手搓 |
| 页面层级 | 手拼返回 + 残留父标题 | `ui.BackButton` + `ui.ForwardButton` + `ui.Breadcrumbs(items, &chosen)` | 注意签名 |
| 筛选与餐型 | 胶囊堆叠 | `ui.Segmented` + `ui.Select` + `ui.RadioGroup` | 餐段多选保留 Toggle |
| 私厨参数 | 嵌套白卡 + `NumberInput` | `ui.Form` + `ui.Field` + `ui.NumberInput` | 步进器本来原生 |
| 仓库/目录网格 | 手写 3 列/2 列 | `ui.GridView` | 自动降列 + Role |
| 新手包地域 | `ui.Grid(4)` 巨型卡 | `ui.Split` 左 `ui.List` + 右详情（或 `ui.Select`） | 8 个同屏 |
| 食材标签 | 灰底胶囊 | 只读 `ui.Badge`；可选 `ui.Toggle` | 不用 `TokenField` |
| 菜式列表 | `ui.GridView` 卡片 | 保留 + 新增 `ui.Table` 模式 | 不删卡片（D4） |
| 菜式详情宏量 | 自绘圆环/圆点/4px 条 | `ui.Meter` ×3 + 色点 | Meter 单色 |
| 菜式详情食材 | 手写行 | `ui.Table`（食材/用量/在库） | 免费 Role |
| 掌序进度 | 圆环 + 4 处文字 | `ui.Progress`（2px） | 只出现一次 |
| 掌序计时 | 绝对定位黑胶囊 | 内联当前行 | 不遮挡 |
| 食历 | 自建 30 格 | `ui.Calendar` + `ui.Split` | 已确认去掉密度点 |
| 空状态 | 月历 + 0 卡 + 空卡 | 单一 `design.EmptyState` + `PrimaryButton` | 提前 return |

---

## 8. 测试与验收

### 8.1 每批次门禁（缺一不可）

```powershell
cd "<repo>"
go build ./...
go vet ./...
go tool mygo vet .
go test ./... -count=1
```

另加两项 UI 专项：

- `go test ./internal/app -run "TestRecipeList|TestPantry|TestSichu|TestScheduleRun|TestFeed" -count=1`
- 截图对比：`test/screenshots/before` vs `after`，尺寸 1280×860 / 1024×720 / 深色。

### 8.2 必须新增/改写的 `ui.NewTester` 用例

| 用例 | 断言 |
|---|---|
| `TestShellBreadcrumbPop` | Push 二级页 → 点面包屑 → `router.Path()` 回父级 |
| `TestShellBackForward` | 返回一次再前进一次，路径往返正确 |
| `TestSidebarTypeahead` | 按 `c` 后焦点/选中到「菜式」 |
| `TestPantryOptionalToggle` | 切换可选条目后，入库数量 +N；再切回 −N |
| `TestSichuFormSegmented` | 餐型切换后 `style` 值变化；餐段仍可多选 |
| `TestRecipeFilterOneRow` | 改主料 Select 后 `recipeView` 重算，计数标签同步 |
| `TestRecipeTableSort` | 点「耗时」表头两次，首行在最快/最慢之间切换 |
| `TestRunProgressSingle` | 步进后 Progress 值变化；页面上「第 %d 步」文本计数 = 1 |
| `TestRunClockHit` | 1024×720 下三行时钟按钮点击都命中（用坐标点击） |
| `TestFeedEmptySinglePanel` | 0 记录时渲染的卡片/面板数量 = 1 |
| `TestFeedSplitDayChange` | 选另一天后右栏列表条目变化 |

### 8.3 性能与动效

- `TestStaticPageRequestsNoFrames` 继续通过（`perf_test.go:108`）。
- 仓库 944 条切换表格/卡片时，首帧构建 ≤ 16ms（`ui.List`/`ui.Table` 虚拟化）。
- 删除 `design.Ring` 后确认没有任何 `AnimationFrameRate` 常驻请求。
- `ReleasePixels` 后 RSS 下降 ≥ 100MB（QW-3）。

### 8.4 三档动效

每个新动效必须走 `design.LevelFor` + `Level.Apply`：

- 完整：保留 `Match` 胶囊、`Stagger`、`Lift`；
- 减弱：`Match` 改为淡入，`Lift` 幅度减半；
- 关闭：`c.DisableAnimations()`，列表直接出现。

**禁止**只判断 `MotionFull` 的新代码。

---

## 9. 提交切分与文档同步

| 提交 | 内容 | 前置 |
|---|---|---|
| `refactor(design): 删死代码、Meter 语义、ReleasePixels` | 阶段 1 | 阶段 0 基线 |
| `refactor(shell): 44px 顶栏、面包屑、离线文案；胶囊零改动` | 阶段 2 | 阶段 1 |
| `refactor(data): Reader/Writer/Admin 零改动替换` | 阶段 3 | 可与阶段 2 并行 |
| `refactor(pantry): GridView、新手包 Split、Form` | 阶段 4 | 阶段 2、3 |
| `refactor(sichu): Form 化配置区` | 阶段 5 | 阶段 2 |
| `refactor(recipes): 单行筛选、Meter、卡片/表格切换` | 阶段 6 | 阶段 3、4 |
| `refactor(schedule): 单一进度、内联计时` | 阶段 7 | 阶段 2 |
| `refactor(feed): 单一空态、Calendar + Split` | 阶段 8 | 阶段 3 |
| `chore: 令牌、Role、AGPL/mpv 文案` | 阶段 9 | 全部 |

每次提交前更新 `docs/进度.md` 一行；不要 amend 已验收提交。

---

## 10. 反清单（明确不做）

| 不做 | 理由 |
|---|---|
| 换掉 `design.Flight` | 跨路由共享元素飞行是框架缺口，`Match` 不跨页 |
| 换掉 `design.Tilt` | 框架没有 hover→spring→3D 的组合件 |
| 换掉 `design.Skeleton` / `Stagger` | 框架无对应组件 |
| 换掉 `internal/assets` LRU | `ui` 只有解码，没有缓存 |
| 换掉自定义 `ModalDialog` | `ui.Modal` 不透明白底，玻璃读不到背后 |
| 删除卡片流只留 Table | 会丢掉飞行；卡片流已是虚拟化的原生用法，Table 作为可选视图即可 |
| 用 `TokenField` 管模板食材 | 删除语义会改模板 |
| 用 `Segmented` 管餐段 | 餐段是多选 |
| 用 `Frameless` | Windows 丢顶边缩放 |
| 把 `Breadcrumbs`/`SearchField` 塞进 `Toolbar` | Toolbar 溢出只认按钮/开关/分段/菜单按钮 |
| 直接改 `ui.Calendar` 的格子 | 框架不支持富内容；只能混合方案 |
| 全量把 228 处硬编码换 `Sp` | 回归风险高；只做 Top3 + 新代码 |

---

## 11. 风险与回滚

| 风险 | 触发点 | 缓解 |
|---|---|---|
| 食历去掉密度点后，整月开饭节奏不够直观 | D3 换 `ui.Calendar`（已拍板） | 左栏底部「本月开饭 N 天」+ 连续天数；右栏按日期倒序 |
| 表格模式失去飞行 | RC-5 | 仅表格模式不飞；卡片模式保留 |
| `Meter` 三条同色 | RC-7 | 保留 6px 色点 + 文本标签 |
| 侧栏胶囊被误改 | D1 已拍板保留现状 | SH-5 只做可选增量；胶囊实现与动画参数零改动，测试锁定 `navPill` 行为 |
| `ui.Calendar` 的日期指针与现有 `feedState` 口径不一致 | FD-2 | 统一用本地日期 `time.Date(y,m,d,0,0,0,0,time.Local)`；加测试 |
| `ApplyTheme` 缓存导致弹层不刷新 | SH-6 | 弹层打开时强制一次 `SetTheme`；加测试 |
| 仓储接口替换漏方法 | DATA-1 | 编译期兜底：`var _ Repository = (*sqlite.Store)(nil)` |
| 视图改太多导致文案回归 | 全阶段 | 每阶段截图 + `ui.NewTester` 断言状态，不测「文字在屏幕上」 |

**统一回滚**：每阶段一个独立 commit；页面批次之间不共享未提交改动。

---

## 12. 最终验收（整体 Definition of Done）

1. 1280×860 下：仓库 ≥18 条、新手包 8 地域同屏、私厨配置 ≤260px、菜式筛选 1 行、食历月历 + 日记同屏且**不再有格内点阵**。
2. `ui.Progress` 只剩「进行中」语义：启动 1、营养摄入 2、个人 XP 1、掌序 1（新增）；静态占比全部 `Meter`。
3. 自绘月历、圆环、圆点、手写网格、`ChipTone` 全部为 0 调用。
4. 菜式页有「卡片 / 表格」切换；默认卡片，表格可排序、可拖列宽。
5. 侧栏滑动胶囊与改动前一致：`navPill` 位置、滑动时长、`ElementTransition` 匹配行为均有测试锁定。
6. `Role(...)` ≥ 12；键盘可从侧栏走到生成配餐/完成一步。
7. 离线文案只剩设置/关于 1 处、AI 页面按需 1 处。
8. `internal/app` 不再出现 `sqlite.Store`；`internal/app` 测试 ≤30s。
9. 四项门禁全绿；`perf_test` 通过；三档动效各有测试。
10. `docs/进度.md` 记录每阶段证据与截图路径。

---

## 13. 已拍板的三件事

三项已确认，方案按下列结论执行，不再保留备选分支：

1. **食历密度点（D3）——去掉。** 改用 `ui.Calendar` 做日期导航，菜名/数量放右栏；自绘 `feedCalendar` / `feedCell` 整段删除。
2. **菜式视图（D4）——保留卡片，并加「卡片 / 表格」切换。** 默认仍是卡片流；表格作为紧凑模式，不做封面飞行。
3. **侧栏胶囊（D1）——保留现状。** 不换 `ui.Sidebar`，`navItem` / `navPill` / `ElementTransition` 与动画参数都不改；徽标、键盘、折叠只作为可选增量。

---

## 附录 · Gemini 原文 → 任务映射（逐句）

| PDF 段落 | 关键句 | 任务 |
|---|---|---|
| 一.1 | 白底大卡片 + 胶囊，缺密度 | §5、PK-1/2、SC-2、FD-3 |
| 一.1 | 新手包 8 块、私厨 4 空卡、食历 30 方块 | PK-3、SC-7、FD-2 |
| 一.2 | 胶囊凑单选组 | SC-3、C7 |
| 一.2 | 输入框 + 圆圈凑步进器 | C1、SC-5（保留 NumberInput） |
| 一.2 | 自定义方格画日历 | C4、D3、FD-2 |
| 一.3 | 离线两处 | SH-4 |
| 一.3 | 父子标题重复 | SH-1 |
| 一.3 | 私厨两句标题语义重叠 | SC-1 |
| 一.3 | 菜式两行 944 | RC-1 |
| 二.1 | 粗蓝条 / 搜索胶囊 / 状态文字 / 手拼返回 | D1、C2/C3、SH-1/4 |
| 二.1 | Frameless + Vibrancy + TitleBar + Toolbar | D2、SH-2、C11 |
| 二.1 | Router + Back/Forward + Breadcrumbs | SH-3、C6 |
| 二.1 | SearchField + Sidebar + Badge | D1、SH-5、C2 |
| 二.2 | 8 地域 Tabs / List + 详情 | PK-3 |
| 二.2 | TokenField | C9、PK-4（不采纳） |
| 二.2 | Form + Field + Switch | PK-5 |
| 二.2 | PrimaryButton / Button | PK-6 |
| 二.3 | 删双层标题、AI 移 Toolbar | SC-1、SH-2 |
| 二.3 | Form 紧凑配置 | SC-2 |
| 二.3 | 餐型 RadioGroup/Segmented | SC-3 |
| 二.3 | 餐段 Segmented | SC-4、C10（保留多选） |
| 二.3 | 人数 NumberInput/Stepper | SC-5、C1/C8 |
| 二.3 | 就餐人 Segmented | SC-6 |
| 二.3 | 配餐依据一行 + Link | SC-7 |
| 二.4 | 20 胶囊混排 + 红黄绿条 | RC-2、RC-3 |
| 二.4 | Segmented 带数字 | RC-2 |
| 二.4 | 4 Select + 当季 Toggle | RC-2 |
| 二.4 | Table / List 替代双列 | D4、RC-5 |
| 二.4 | Table 排序/列宽/键盘 | RC-5 |
| 二.4 | 7 做法 Tabs/Select | RC-6 |
| 二.4 | Meter 替代圆环/圆点 | RC-7、C12 |
| 二.4 | 食材 Table | RC-8 |
| 二.5 | 冗余、4 次进度、黑条、下一步 | RUN-1…RUN-4 |
| 二.5 | 砍大卡与圆环 | RUN-1 |
| 二.5 | Progress + 总耗时 | RUN-2 |
| 二.5 | 移除下一步/黑框，倒计时内联 | RUN-3/4 |
| 二.6 | 30 方块、首行裁切阴影 | FD-2、G-G1 |
| 二.6 | 零记录三重空态 | FD-1 |
| 二.6 | godiff emptyPanel + PrimaryButton | FD-1 |
| 二.6 | Calendar + Split | D3、FD-2、C4/C5 |
| 三 | 8 行速查表 | §7 |
| 三 | 删自绘、代码 -40% | D7、CL-6 |
