# 食匣 · FoodBox-Go

用 Go + [MyGo](https://github.com/egoist/mygo) 原生 UI 重写的「食匣」桌面版：
离线单机的家庭厨房应用，**全功能对齐**原 Flutter 版。

- 原生 GPU 绘制，无 WebView、无 Flutter 引擎
- Windows 桌面优先，代码保持 macOS / Linux 可编译
- 复用原 FoodBox 的 944 道菜式、1,000 条食材目录与 1,000+ 图片资产
- 本地 SQLite（10 张表 + app_meta），无账号、无云端、无网络依赖
- 三档动效（完整/减弱/关闭），跟随系统「减少动态效果」

## 功能

| 模块 | 能力 |
|---|---|
| 仓库 | 主菜区/底料架分区、临期先吃、数量档位与精确数量、1,000 条系统目录、自建条目、盘点核销、8 地域新手包、购物清单 |
| 菜式 | 944 道菜虚拟列表、搜索、三维筛选、当季筛选、排序、现在能做/差一点/差太多、详情、编辑器、版本组 |
| 视频 | Bilibili 直链解析（WBI 签名、重试、多 P、CDN 备用地址）与内嵌 mpv 播放；失败时全屏 WebView 回退 |
| 掌序 | 待做队列、备料清单、灶台/案板并行排程（golden）、歌词流执行、暂停计时、挂起托盘、会话恢复、结算扣库存与开饭日记 |
| 私厨 | 两菜一汤 / 一碗端、身体档案、同桌成员、忌口与疾病硬约束、确定性本地策略、AI（DeepSeek，可配 Key，失败回退本地）、每日配额 |
| 营养 | 1,026 条食物成分表、30+ 家庭单位换算、BMR/TDEE/宏量目标、7 日摄入、嘌呤负荷、食材贡献 |
| 食历 | 月历、开饭日记（按天唯一）、年度回顾（Top5/月度/菜系/连续天数） |
| 我的 | 昵称/等级（进度条）、收藏、做过的菜（列表+统计双视图、筛选排序）、最近下厨 |
| 设置 | 动效持久化、AI 与配额、十表备份导出/导入、帮助、关于 |
| 引导 | 首次启动问卷（灶台/锅具/籍贯/口味/忌口/家庭），只出现一次，可跳过 |

## 开发

需要 Go 1.27+（把 `go` 放进 PATH，或使用你自己的工具链目录）。

```powershell
go tool mygo dev      # 开发运行（热重启）
go tool mygo build    # 打包到 build/（exe + NSIS 安装包）
go test ./...         # 单元测试 + 无窗口 UI 测试
go vet ./...          # 静态检查
go tool mygo vet .    # MyGo 生命周期检查
go tool mygo doctor   # 检查环境（NSIS / signtool / WebView2）
```

资源与证据：

```powershell
pwsh -File tools/sync-assets.ps1   # 从原 Flutter 项目同步数据/图片并重钉哈希
pwsh -File tools/screenshot.ps1    # 用 ui.Tester 重新生成 test/screenshots
```

## 下载与自动更新

普通用户不需要安装 Go：到 [Releases](https://github.com/gij072858-cmd/foodbox-go/releases)
下载 `foodbox-setup-<version>-amd64.exe`，用安装版装到当前用户目录即可。

应用内置 MyGo 的签名更新器（`plugins/updater/native`）：

- 启动约 10 秒后检查一次，之后每天检查 GitHub 的最新 Release；
- 新版本会弹出更新窗口，可安装、稍后提醒或跳过；
- 用户勾选自动下载后，更新在下次启动生效；菜谱、记录和设置保留。

维护者发版：改 `mygo.json` 的 `version`、在 `CHANGELOG.md` 写对应小节，提交后打 tag
并推送：

```powershell
git tag v0.1.1
git push origin v0.1.1
```

`.github/workflows/release.yml` 会用仓库 Secret `MYGO_UPDATER_PRIVATE_KEY` 签名、
构建安装包与更新清单，并发布 Release。私钥绝不能进仓库；丢失后老用户无法再自动更新。

## 架构

```text
internal/
├── app/        窗口、壳、路由、各页面（每功能一个 *_views.go）
├── domain/     纯领域模型与枚举
├── repository/ 仓储接口
├── data/
│   ├── sqlite/ 唯一 import plugins/sqlite 的包（schema/迁移/仓储实现）
│   ├── seed/   JSON 种子导入
│   └── backup/ 备份格式 v3 + 旧 v2 导入
├── assets/     go:embed + 图片 LRU + 资源哈希
├── design/     tokens、组件 kit、三档动效
├── services/   matcher / schedule / settlement / nutrition / sichu / ai / season / review
└── features 语义上并入 app 的各页面文件
```

纪律：页面不碰 SQLite；领域模型不依赖 UI/DB；DB/网络/解码在 goroutine，
结果经 `Window.Update` 回主线程；视图只读快照，944 行用 `ui.List` 虚拟化。

## 验证基线

```powershell
go build ./... && go test ./... -count=1 && go vet ./... && go tool mygo vet .
go test ./internal/app -run TestMatchPerf944 -v          # 944×200 匹配 < 50 ms
go test ./internal/data/seed -run TestImportPerformance  # 首启导入 < 10 s
go test ./internal/assets -run TestAssetsLock            # 资源哈希
```

CI：`.github/workflows/ci.yml`（windows-latest，Go 1.27.x）。

## 文档

- [方案：MyGo 原生 UI 重写](docs/方案-MyGo原生UI重写.md)
- [进度](docs/进度.md)
- [M7 全量审计](docs/验收-M7-全量审计.md)
- [并行施工任务书](docs/施工-M3-M7-任务书.md)
- [参考实现：Cloudy 源码分析](docs/参考-Cloudy源码分析.md)
- [交接：给下一个 Agent](docs/交接-给下一个Agent.md)

## 来源与许可证

本仓库以 **AGPL-3.0-only** 发布，全文见 [LICENSE.md](LICENSE.md)。选它是因为下面第二类来源
Cloudy 是 AGPL-3.0，而我们逐字复制了它的代码单元——本仓库因此是 AGPL 衍生作品，
对外分发必须随附对应源码。

- 产品功能与数据资产来自原 FoodBox 项目（本仓库不改动原项目）。
- **Kero（MIT）** — 主题算法整段复制，声明保留在 `internal/design/kero_theme.go` 文件头。
- **Cloudy（AGPL-3.0）** — [nya-owo/cloudy](https://github.com/nya-owo/cloudy)。逐字复制的代码单元，
  每个都写在承接文件的头部注释里（源文件 + 行段）：
  - `internal/design/gravity.go` ← `internal/app/ui_cover.go`（`glareSource`/`glareShader` 51-77、
    倾斜块 184-248、`clamp1` 287）
  - `internal/design/flight.go` ← `internal/app/ui_expand.go`（`lerpRect`/`easeRange` 50-59、
    `flightEnd`/`flightTouch` 64-67、`coverFlight` 262-270、`landCover` 330-343、
    `coverFlightView` 347-388）
- MyGo 框架本身为 MIT。
