# 学习手记：从 egoist 的 MyGo / Cloudy / Kero 里偷师

> 参考源码：
> - MyGo 框架：`$(go env GOMODCACHE)\github.com\egoist\mygo@v0.7.4`
> - Cloudy 音乐播放器：`<cloudy-source>`（AGPL-3.0，只读参考）
> - Kero 终端：本机安装版；可学的更多是「一个窗口如何把状态、输入、动效组织干净」
>
> 本文只记录**设计思路**与 FoodBox 的落地方式，不复制 Cloudy 代码。

---

## 1. 一句话总结 Cloudy 的动效系统

Cloudy 不是「给每个组件配一条动画」，而是三层：

1. **令牌层**：3 个时长（跟手 200ms 弹簧、浮层 280ms、换页 340ms）+ 1 条共享曲线
   `cubic-bezier(0.22, 0.8, 0.24, 1)` + 三档运动（完整 / 减弱 / 关闭）。
2. **空间叙事层**：页面按「深入 / 返回 / 同级」方向进入；浮层从触发点缩放；卡片封面飞到详情；
   列表首屏逐行入场；数字逐位滚动。
3. **帧预算层**：能只重绘（`AnimatePose` / `Painter`）就不重建视图；能 15/30fps 就不 60；
   动画结束就停止请求帧；被覆盖的页面不构建。

FoodBox 这次先落地了第 1、2 层里成本最低、收益最高的部分，并把第 3 层写进组件约束。

---

## 2. 十条最值得学的设计原则

### 2.1 「能 pose 就不 layout」

- `AnimatePose` 只重绘，不重建视图；`ElementTransition` 会重新布局。
- 悬停抬升、按压回弹、图片飞行、图标旋转，全部走 pose。
- FoodBox：`design.Lift` 统一了卡片/按钮的悬浮与按压。

### 2.2 方向性页面转场，且没有 Exit

- 深入：从右 34px；返回：从左 24px；同级：从下 10px；340ms。
- **不写 Exit**：两页同时存在会互相透视，读起来像哪一页都不是。
- FoodBox：`router.Transition` 跟随三档动效；`MotionLite` 只淡入；`MotionOff` 直接切。

### 2.3 三档动效必须有真实语义

- 关闭 = 连 hover / press / 页面切换都停，不是只停「大动画」。
- 减弱 = 保留颜色/淡入，去掉位移、飞行、stagger。
- FoodBox：`design.Level.Apply(c)` 在视图构建前调用 `c.DisableAnimations()`；
  `RouterTransition()` 返回 `TransitionNone / Fade / Auto`。

### 2.4 首屏 stagger，长列表不动

- 前 14 行，每行延迟 24ms，280ms 内从下方 10px 淡入；返回时不重复演出。
- FoodBox：`design.Stagger` + `design.StaggerRow`，用在仓库、菜式、目录列表。

### 2.5 骨架屏与最终布局同构

- 1600ms 周期、1100ms 扫光、260px 光带，所有骨块共享同一时钟。
- 文本骨块高 ≈ 字号 × 0.78，圆角 ≈ 高 × 0.32；行宽用固定「伪随机」数组错落。
- FoodBox：`design.Skeleton` / `SkeletonFill`；私厨生成中显示三张方案骨架卡。

### 2.6 图片淡入，而不是「啪」地出现

- 图片未就绪时占位；就绪后 320ms 淡入，占位同步淡出。
- 已经就绪的图片在首帧直接显示，不做二次闪入。
- FoodBox：`design.ImageIn` + `design.DrawImage`；仓库卡片与目录行已接入。

### 2.7 数字滚动是最便宜的「高级感」

- 数字变化时只滚动变化的位，单位位先动，形成进位感；420ms 弹簧，30ms/位。
- FoodBox：统计卡与仓库计数使用 `ui.ContentTransition{Numeric: true}`。

### 2.8 颜色用 alpha 派生，不堆硬编码灰

- hover / press / 分隔线 / 阴影都用黑白 alpha，天然适配明暗。
- 卡片阴影只有两档：静止 `0,1,4,-1`，悬浮 `0,12,28,-8`。
- FoodBox：`design.Wash` / `RestShadow` / `HoverShadow`。

### 2.9 反馈要有时效，也要有语气

- 失败红、完成绿、其余强调色；4 秒后自动消失，不变成常驻家具。
- FoodBox：`App.View` 监听 `notice` 字符串变化并调用一次 `c.Toast`；超时、堆叠、
  淡入淡出都由框架的 toast host 处理，页面不再各自画提示。

### 2.10 桌面应用要有键盘

- `Cmd/Ctrl+F` 聚焦搜索；`Cmd/Ctrl+1…7` 切侧栏；`Esc` 关弹层。
- FoodBox：`App.shortcuts`；搜索框绑定 `ui.Handle`，帮助页同步更新。

---

## 3. 本次已落地清单

| 主题 | 文件 | 说明 |
|---|---|---|
| 字号/圆角/间距阶梯 | `internal/design/tokens.go` | `FontDisplay…FontCaption`、`RadiusSmall…RadiusDialog`、`Space1…6` |
| 三档动效语义 | `internal/design/motion.go` | `Level.Apply`、`RouterTransition`、`Stagger*` |
| 统一悬浮/按压/颜色过渡 | `internal/design/components.go` | `Wash`、`Lift`、`RestShadow`、`HoverShadow` |
| 按钮层级 | `internal/design/components.go` | `ButtonPrimary/Secondary/Danger/Ghost`、`IconButton` |
| 骨架屏 | `internal/design/components.go` | `Skeleton`、`SkeletonFill`（扫光同步、空闲休眠） |
| 图片淡入 | `internal/design/components.go` | `ImageIn`、`DrawImage`、`FadeImage` |
| 逐行入场 | `internal/design/components.go` | `Stagger`、`StaggerRow` |
| 反馈条 | `internal/app/app.go` | 框架 `c.Toast`，notice 变化时调用一次 |
| 内容列与列表对齐 | `internal/app/shell.go` | `pageFrame`、`listBody`；修正 4 值 `Padding` 顺序 |
| 壳层 | `internal/app/shell.go` | 顶部标题与内容同线、宽屏居中、移除 M7 与动效探针、全局搜索跳转 |
| 仓库 | `internal/app/pantry_views.go` | 统计/操作/搜索分行、卡片悬浮、图片淡入、空态带「清除筛选」 |
| 菜式 | `internal/app/recipes_views.go`、`recipe_mark.go`、`recipe_hero.go` | 分类色条+图标底（非占位图）、76px 行、详情数据卡、筛选条收敛、卡片 stagger、空态带操作 |
| 菜式详情 | `internal/app/recipes_views.go` | 大封面带菜系角标、危险操作红色、共享元素飞行 |
| 食历 | `internal/app/feed_views.go` | 日历格压缩到 64px、悬浮过渡、数字滚动 |
| 私厨 | `internal/app/sichu_views.go` | 生成中骨架卡 |
| 设置 | `internal/app/settings_views.go` | 外观（跟随系统/浅色/深色）+ 动效；去掉卡片套卡片 |
| 启动 | `internal/app/startup.go` | Logo 一次性入场（Keyframes） |
| 快捷键 | `internal/app/app.go` | `Cmd/Ctrl+F`、`Cmd/Ctrl+1…7` |
| 深色证据 | `test/screenshots/m2-pantry-dark.png` | 深色模式首次截图归档 |
| 滚动边缘 + 粘性头部 | `internal/app/shell.go`、`recipes_views.go` | 内容压边才淡出；详情滚过 120px 出现迷你条 |
| 仓库封面飞行 | `internal/app/pantry_views.go` | `pantryFlight` + `Match`，卡片封面/标题飞到详情 |
| 掌序执行页 | `internal/app/schedule_views.go` | 进度环 hero、当前步骤大字、下一步预览、更多操作折叠 |
| 编辑器分组 + 保存栏 | `internal/app/m3_recipe_editor.go` | 四组卡片、底部固定保存栏、Switch/Checkbox 替代灰色 Toggle |
| 引导步骤化 | `internal/app/m6_onboarding.go` | 厨房/家人/口味/确认四步、进度点、固定上一步/下一步 |
| 图表生长 + QA | `internal/design/components.go`、`motion_qa_test.go` | BarChart/Meter/Ring 一次性生长；空闲不请求帧、转场逐帧差分 |
| Liquid Glass | `internal/design/components.go`、`shell.go`、`pantry_views.go`、`m3_recipe_editor.go` | `PaneMaterial`/`BarMaterial`；弹层、粘性条、保存栏、滚动边缘 |

---

## 4. 菜品标识：从「假照片」到「有意义的色 + 真实数据」

原 Flutter 版有意退役了菜品照片；`manifest` 里只有 36 张分类图和 1000 张食材图。
直接拿 `Region` 去取分类图，会出现「青椒肉丝显示莲藕」「鱼香肉丝和麻婆豆腐同一张图」——
**没有照片好过错误的照片**。

第一代替代方案（`recipeCover`，已删除）用「光」补位：分类色渐变 + 菜名哈希第二层色 +
左上窗光 + 首字作印。它解决了「不重复」，但留下更大的问题：**这块色占了照片的位，
却没有照片的内容**。列表里它是一张 92px 的空海报，详情里它是一块 320×220 的大字报，
用户一眼就知道那不是图。

第二代（M9）把「身份」和「内容」拆开：

1. **身份交给色调**：14 个 `MainCategory` 收敛成 5 个 `design.DishTone`
   （荤 / 水产 / 蔬 / 蛋豆 / 主食），色相沿用旧封面但提高饱和度 —— 淡彩在 3pt
   宽度上等于灰色，这是第一版改造截图里踩出来的结论。
2. **位置交给文字**：列表行删掉封面块，行高 112 → 76，一屏多看约 3 行；
   3pt 色条贴在卡片左缘（`Clip` 才不会把圆角切平），图标底只做 34px。
3. **详情那块 320×220 换成数据卡**（`recipe_hero.go`）：宏量三弧环 + 每 1 人份 kcal、
   必需食材在库点、嘌呤 mg 与档位。环用 `ui.Path.Circle + Trim`（`design.Ring` 的画法），
   静态绘制 —— 停稳后不请求帧，帧预算测试守着。

两个口径必须同时给：**944 道的 Coverage 全是 full，但 194 道（20.6%）嘌呤估算为 0 mg**，
而按「最坏食材」仍分档 —— 红烧肉 0 mg 是中档，辣炒花蛤 0 mg 是高档。只写 mg 会骗人，
只写档会丢量。同理 `Catalog.Recipe` 返回的是**每 1 人份**（已除 `BaseServings`），
文案写「每 1 人份」，不能写「整道」。

另外两条数据事实决定了不做什么：只有 128 道（13.6%）标了当季，所以不做 12 格月份条；
目录是异步加载的，所以卡面在未就绪时写「营养目录读取中」，绝不先把数字画出来。

图标不新开一套：荤用 `Flame`、主食用 `Bowl`、蔬用 `Leaf`，只有 `Fish`、`Egg`
是为这次改造新绘的描边图标。飞行也从「整块封面」降级为「图标底」——
`recipeFlight{Part: "mark"}` 让卡片那枚 34px 的 tile 飞到详情卡头，比搬一张假照片诚实。

---

## 4.5 玻璃：学的是材质分层，不是加一层滤镜

Cloudy 的玻璃分两级：

- **普通浮层**（菜单/弹窗/Toast）走 `pane()`：玻璃或纯色兜底、0.5px 发丝线、
  统一 `0 18 44 -12` 阴影；深色玻璃 tint `rgba(14,14,18,.62)`，浅色 `rgba(250,250,252,.58)`。
- **播放栏**才用 `ui/shader` 自写 `barGlass`：backdrop 采样、Snell 折射、红蓝分离、
  边缘高光、亮度压缩；CPU 上直接退回插件玻璃，因为解释执行 shader 太慢。

FoodBox 用的是 MyGo v0.7.4 自带的 `plugins/glass`（和 Cloudy 的插件玻璃同源）：

- `design.PaneMaterial(p)`：弹层玻璃，深色 `rgba(14,14,18,.50)`，浅色 `rgba(250,250,252,.30)`。
- `design.BarMaterial(p)`：粘性条/保存栏，比 pane 略不透明，保证文字可读。
- 滚动边缘直接用 `glass.ScrollEdge{Background: p.Bg, Bottom: !top}`，不再手绘渐变。
- **关键坑**：`ui.Modal` 会给 panel 铺一层 `t.Background`，玻璃采样到的是这层白底，
  所以弹层看起来还是实色。要真玻璃，必须用 `ui.DialogBase` 自己铺半透明 backdrop + panel，
  玻璃才能读到背后的页面内容。仓库条目详情、快捷编辑、自建条目、成员、结算、探针都已改。

材质分层比「到处加玻璃」重要：内容页保持实色，只有**浮在内容之上**的条/层才用玻璃。

---

## 4.6 视频：MyGo 还没有系统解码器

菜式种子的 914 条来源链接里，880 条是 B 站、34 条是小红书，都是**网页**，不是 mp4。
MyGo v0.7.4 的 `ui.Video` 只内置 `NewGIFDecoder`；H.264/HEVC/VP9/AV1 的系统解码器
「not part of MyGo yet」。所以应用内播放只有一条路：`ui.WebView` 加载来源页。

FoodBox 的做法（`internal/app/video.go`）：

- 菜式详情「看视频」→ `Window.NewWebView(URL)` → `ui.DialogBase` 里放 `ui.WebView`；
- 关闭/点背景/Esc 时 `Destroy()`，避免 WebView 在后台继续解码；
- 无窗口（测试）或 WebView 创建失败时回退到系统浏览器。

这是项目「无 WebView」原则的明确例外：只为外部视频页开一个浮层，不把应用 UI 搬进 WebView。
等 MyGo 接上系统解码器后，可以换回 `ui.Video` + 本地/直链视频。

---

## 4.7 侧栏胶囊：MyGo 没有，但原语有

`ui.Sidebar` 的 `SidebarItem` 选中时只是 `Background(t.Accent)`，没有滑动；框架里也没有
“导航胶囊”组件。Cloudy 的胶囊是它自己在选中项里放一个 `ui.Box`，用
`ElementTransition{Match: navPillMatch{}}` 做的。

FoodBox 照这个思路重写了侧栏（`shell.go`）：

- 用 `ui.ButtonBase` 建每一项，`ui.FocusGroup(ui.Vertical)` 保留方向键导航；
- 选中项里先放一个 `Fill()` 的胶囊（在流内，先画），图标/文字用绝对定位盖在上面；
- 胶囊 `Transition{Animation: &design.Pane, Match: navPillMatch{}}`，从上一项滑过来；
- 关闭动效时 `DisableAnimations` 会把它直接落位。

所以答案是：**MyGo 没有现成的侧栏胶囊，但 `Match` 是现成的**；我们用的是原语，不是重造动画引擎。

---

## 5. 下一批还可以学的

第一、二批已经把「低成本高收益」的部分做完了；剩下的更偏体验深水区：

1. **Now Playing 式 hero morph**：从底栏/卡片矩形插值到全屏；原则是「移动盒子，不是 transform」，
   被覆盖页面停止构建。适合购物车底栏 → 结算抽屉。
2. **滚动歌词式「滞后量」**：offset 一帧到位，只把视觉滞后量给内容；用户滚动立即接管。
   适合掌序执行页的歌词流自动跟随。
3. **封面取色**：把菜品图缩到 24×24、HSV 18 色相桶、`s²v` 加权，派生详情页背景/强调色。
4. **像素级动效 QA**：目前有逐帧差分与空闲帧预算；还可以像 Cloudy 一样给封面染色、
   逐帧追踪重心，断言「飞行途中不消失、不同时出现两个副本」。
5. **雾/流动背景**：详情页头图或烹饪中的背景；玻璃已接入，雾 shader 还没做。
6. **迷你窗口 / 托盘浮窗**：掌序进行中缩成小窗，计时和当前步骤常驻。
7. **图表交互**：柱状图 hover tooltip、图例过滤、堆叠；目前只有生长动画。
8. **真实窗口 automation 截图**：`ui.Tester` 之外，用 MyGo automation 插件跑真实窗口。
9. **细节层**：tooltip、焦点环、键盘提示、危险操作二次确认的统一封装。
10. **引导页的动效**：目前是步骤切换；可以加 60ms 错峰入场与方向性滑动。

---

## 6. 踩坑记录（下次别再踩）

1. **MyGo 的 4 值 `Padding` 是 `top, right, bottom, left`**，不是 CSS 简写里的
   `top, horizontal, bottom`。`Padding(0, 0, 0, 12)` 是「左边 12」，不是「底部 12」。
   仓库/菜式列表与表头的 28px 错位，根因就在这里。
2. **`ElementTransition.Enter` 只在父容器上一帧已存在时播放**；整页/对话框第一次出现不会演。
   需要首帧入场时用 `Keyframes`（如启动 Logo）或手写时间函数。
3. **`ui.Modal` 关闭时整棵子树不再构建**，所以 `Exit` 不会播；要退出动画得自己保留关闭中的状态。
4. **`AnimateColor` 在换主题时会从旧颜色过渡到新颜色**。截图/测试要 `Advance` 过动画，
   否则会拍到「浅色卡片 + 深色文字」的中间帧。
5. **`AnimatePose` 的 `Scale=0` 表示保持原尺寸**，不是缩到 0；`Fade` 0 才是透明。
6. **List 的行本身不参与 `ElementTransition` 的进入/退出**；行级动画要放在行内容上。
7. **`ReduceMotion` 下颜色和 `Pose.Fade` 仍会动**，位移类才瞬变；所以「关闭」不能只靠系统偏好，
   还要 `c.DisableAnimations()`。
8. **`Padding` 之后不要再用 `Background` 画「看起来像输入框」的整行**——设置页的
   `Toggle` 目前就是这样，后续可以换成真正的开关行。

---

## 7. Kero 能学什么：状态机，而不是组件库

Kero 本机只有安装版（`<Kero-install-dir>`），没有源码；
但它的 CLI/技能文档已经把设计讲清楚了：

- **会话与窗格是两回事**：session 是长期存在的工作上下文，pane 是它里面的一个视图。
  一个 session 可以同时有 shell、测试、浏览器、另一个 agent。
- **状态词汇统一**：`working / waiting / idle / error / exited`。用户不需要读终端，
  只看状态就知道哪里要人。
- **后台持续 + 可恢复**：关掉窗口不等于杀掉工作；`kero history` / `resume` 把上下文接回来。
- **并行是默认的**：`split` 一条命令就能把测试放到下面跑，而不是让用户开第二个终端。

映射到 FoodBox 的「掌序」：

- 一桌菜 = session；灶台/案板/备料 = pane。
- 每个步骤应有统一状态：等待下锅 / 进行中 / 需要人（翻炒、尝味）/ 出错 / 完成。
- 执行页的「暂停/挂起托盘/会话恢复」已经在做后台持续；下一步可以把状态词汇统一成
  一套颜色与文案，让用户一眼看到「哪个灶眼在等我」。
- Kero 的 `wait --for waiting,error` 对应掌序的「提醒我什么时候该翻面/该关火」，
  比单纯倒计时更贴近做饭。

---

## 8. 继续阅读的入口

- 动效令牌与门控：Cloudy `internal/app/ui_expand.go:16-40`
- 页面方向转场：`ui_expand.go:193-214`
- 列表 stagger：`ui_playlist.go:323-357`
- 骨架屏：`ui_skeleton.go:13-96`
- 图片淡入：`ui_kit.go:173-196`
- 数字滚轮：`ui_comments.go:107-157`
- 对话框手写动效：`ui_expand.go:228-257`
- 封面飞行：`ui_expand.go:259-395`
- 歌词滞后：`ui_nowplaying.go:667-737`
- 帧预算测试：`internal/app/perf_test.go`、`glassbar_test.go:57-67`、`light_test.go:60-74`
- MyGo 动画语义：`ui/docs/ui/motion.md`、`ui/docs/ui/transitions.md`
