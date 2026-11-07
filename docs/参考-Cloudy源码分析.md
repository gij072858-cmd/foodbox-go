# 参考实现：Cloudy（mygo-cloudmusic fork）源码分析

> 本地路径：`<cloudy-source>`
> 许可证：AGPL-3.0
> 规模：145 个 Go 文件 / 约 3.05 万行（非测试约 2.18 万），57 个测试文件 / 约 8 千行
> 技术：Go 1.27.3 + MyGo（固定在 2026-10-27 的 commit）

Cloudy 是原 `egoist/mygo-cloudmusic` 的 fork，证明 MyGo 原生 UI 可以承载商业级桌面应用。本项目把它作为**架构与手感参考**，实现按 FoodBox 的数据结构自行重写。

## 1. 动效分层（`internal/app/ui_expand.go`）

```go
motionEase = ui.CubicBezier(0.22, 0.8, 0.24, 1)
motionFast = ui.Animation{Spring: ui.SpringOptions{Duration: 200 * time.Millisecond}} // 跟手
motionPane = ui.Animation{Duration: 280 * time.Millisecond, Ease: motionEase}         // 弹层
motionPage = ui.Animation{Duration: 340 * time.Millisecond, Ease: motionEase}         // 换页
```

- `motionOff / motionLite / motionFull` 三档，跟随系统 `ReduceMotion`。
- 页面用 `ElementTransition{Enter/Exit: &ui.Motion{...}}`，按前进/后退方向决定进入方向。
- 弹层用 `paneMotion`，从触发点缩放 0.86 进入。
- 对话框因父容器是新的，Transition 不生效，手工用 `AnimateTo` 进出。

## 2. 微交互（`internal/app/ui_kit.go`）

- 圆形按钮：hover 背景 120ms 渐变；按下 `AnimatePose` 缩到 0.88，`ui.Snappy`。
- 播放/暂停图标：两枚图标用弹簧 `340ms / Bounce 0.3` 交叉缩放淡入淡出。
- 卡片：hover 上浮 5px、放大 1.015，阴影加深；全部走 `AnimatePose`，只绘制不重建视图。

## 3. 卡片→详情飞行（`ui_expand.go` `flights()`）

把封面、标题、进度条、播放按钮的矩形从 bar 的 `barSpot` 插值到页面 `npSpot`：

- `lerpRect` 插值位置与尺寸；
- 圆角、阴影、字号、颜色同步插值；
- 两端各自隐藏原元素，交接在最后 0.5% 完成；
- `flown()` / `lifted()` 控制交接时机。

FoodBox 的「菜式卡片 → 详情页」「仓库条目 → 详情」可直接套这个模式。

## 4. 歌词流（`ui_nowplaying.go`）

关键点：**不要直接用 `ScrollTo`**。它每帧移动都会导致下一帧重建视图，而且弹簧在最后 1px 会磨半秒。

Cloudy 的做法：

- 目标行位置一帧内直接设到 `ScrollState.Y`；
- 用 `lyricGlide`（临界阻尼 `(a + b·t)·e^(−ωt)`，ω=10）只把视觉滞后量加给歌词行；
- 用户一滚动立刻接管；
- `ReduceMotion` 时直接关闭。

这是 FoodBox「掌序歌词流」最值得参考的实现。

## 5. 光效（`ui_light.go`）

- 已唱歌词复制一层，`Blur(size*0.22)`，叠在原文字下方；
- 逐字 `wordLit` 点亮，长音词 `wordSwell` 膨胀；
- 进度条光用 `p.Shadow` + 扩散，不额外贴图。

## 6. 骨架屏（`ui_skeleton.go`）

灰块用 `Draw` 绘制，扫光是一条 260 DIP 的渐变带，1600ms 周期，`AnimationFrameRate(60)`；`ReduceMotion` 时只保留灰块。

## 7. 玻璃（`ui_barglass.go`）

不是简单 `glass.Glass{}`，而是用 `ui/shader` 写胶囊玻璃：

- `backdropSample` 读背后内容；
- Snell 定律算边缘折射；
- 红蓝分离模拟色散；
- 边缘高光、亮度压缩；
- CPU 上不跑 shader（解释执行太慢），退回 `glass.Paint`。

## 8. 背景（`backdrop.go`）

- 静止时画进 1/8 窗口大小的 `ui.Canvas` 再放大；
- 开雾时 GPU 上半窗口、30fps 跑 5 阶 FBM 噪声 + 尘埃 + 低音驱动；
- 换歌时旧背景留在另一个 Canvas 上做 700ms 交叉淡出。

## 9. 图片管线（`artwork.go`）

- 64MB LRU；
- 信号量限制 8 个并发解码；
- `weak.Pointer` 保留刚淘汰但仍在用的封面；
- 解码后 2 秒 `ReleasePixels`；
- 失败封面 30 秒退避；
- 同时算主色与模糊背景。

FoodBox 的 1040 张 WebP 直接套这套思路。

## 10. 中文字体（`fonts.go`）

必须：

```go
_ "github.com/egoist/mygo/locales/zh-Hans"
```

否则 Windows 会按英文 App 选择日文字体渲染汉字，出现「一个字一个字体」。默认字体建议 `Noto Sans SC, Microsoft YaHei UI`。

## 11. 帧预算（`internal/app/perf_test.go`）

- 播放栏封面旋转 ≤16 帧/秒，视图构建 ≤4 次；
- Now Playing 静止 ≤3 帧/秒；
- 背景淡出后必须停止请求帧。

**动画结束就停止请求帧**，是高级动画不卡的根本。

## 12. 测试哲学（`AGENTS.md`）

- 只测会坏的行为：点击、按键、拖动、滚动导致的状态变化；
- 不测「标签显示在屏幕上」这种重复视图代码的断言；
- 不依赖墙上时钟，用 `ui.Tester` 的虚拟时钟；
- 视觉用截图看，不断言像素（除覆盖层等）。

## 13. Cloudy 没有、FoodBox 必须自建

- SQLite schema、迁移、种子导入、备份/恢复；
- 仓储层与领域模型；
- 菜式匹配、排程引擎、营养计算、私厨策略；
- Android/移动端；
- 它用手写 history 路由，FoodBox 优先用 `ui.Router`。

## 14. 许可证边界

Cloudy 为 AGPL-3.0。本项目为期末作业，无商业发布计划；默认只参考思路、自行重写。若复制其代码，需在项目内保留 AGPL-3.0 LICENSE 与来源说明。
