# assets/images/icons/ —— 代码常量级图标

命名：`icon_<用途>.webp`（如 `icon_xiaohongshu.webp`）。

> **授权结论（第二批 B2-REL-03）**：三张平台图标（哔哩哔哩 / 小红书 / 抖音）是各平台
> **官方标识**，权利归各自权利人；此处仅作「这道菜参考自哪个平台」的**指示出处式引用性使用**，
> 不用于食匣自身品牌、不暗示官方合作或已获授权。**对外分发 / 上架 / 收费前必须复核**：
> 要么换回自绘简化符号（见下文「怎么换图」），要么先取得授权。当前为个人版、不分发。

## 现在有什么（T6.13 平台来源图标）

| 文件 | 指代 | 画面 | 来源 |
|---|---|---|---|
| `icon_xiaohongshu.webp` | 小红书 | 红底白色「小红书」字标 | **官方应用图标**（App Store 中国区，Xingin） |
| `icon_douyin.webp` | 抖音 | 黑底青/品红错位音符 | **官方应用图标**（App Store 中国区，Beijing Douyin Technology） |
| `icon_bilibili.webp` | 哔哩哔哩 | 粉底小电视 + bilibili 字 | **官方应用图标**（App Store 中国区，bilibili） |
| `icon_self.webp` | 食匣自研 | 米白底 + 墨色碗 + 三缕档案红热气 | **自绘**（食匣没有外部平台可指代） |

- **三张平台图标是各平台的官方标识，权利归其权利人所有**。这里是**指示出处**的引用性使用：
  只用来标「这道菜的做法参考自哪个平台」，不用于食匣自身品牌、不做成"官方合作/已授权"的观感。
  2026-09-15 由本人拍板：从「自绘简化符号」改为直接用官方标识（识别度更高）。
- **使用条件（务必留意）**：当前是**个人版、不分发、不上架**，风险面很小。**一旦要对外发布或收费**，
  这三张要么换回自绘简化符号（那套画法不在当前文件里，去 git 历史取：
  `git show 769965d -- app/tool/gen_platform_icons.dart`，把三张图加回 `_icons` 重跑即可），
  要么先取得授权 —— 见《阶段六-优化升级建议.md》附录 B §B.2「使用平台 logo」「商业化放大」两行。
- **佚名家常菜档不配图**：说不清出处的菜挂一枚图标反而是误导，`FbSourceLine` 遇到
  `RecipePlatform.anonymous` 只留文字。`sichu`（私厨 AI 生成）同样不挂平台图标。

## 怎么换图 / 再生成

平台图标（官方标识）：从 **iTunes Search API** 取该 App 的 `artworkUrl100`，把尺寸段换成
`512x512bb.png` 下载，覆盖 `tool/inbox/icons/icon_<平台>.png`：

```
https://itunes.apple.com/search?term=抖音&country=cn&entity=software
```

自研图标（自绘）：`dart run tool/gen_platform_icons.dart` 画源图到 `tool/inbox/icons/`。

两条路最后一步相同：`cd app && dart run tool/process_images.dart`
（规格化 512 · 1:1 · q80 · ≤40KB、8% 留白，落成本目录的 webp）。
换图**不需要**改代码、不需要动 `pubspec.yaml`（本目录已整体声明为 asset）。

## 谁来引用

`lib/widgets/fb_source_line.dart` 的 `kPlatformIconAsset` 常量（平台 → 字面 asset 路径）。
这批图**不进 `manifest.json`**（manifest 服务「按食材名 / 菜式名解析」的数据图，
`gen_image_manifest.dart` 从不生成 `icons` 段），所以**两个工具都不以「清单里有没有它」当判据**：
`gen_image_manifest` 的孤儿图告警显式跳过 `icons/`；`validate_assets.dart` 自 2026-10-19
（第二批 R2 封面退役）起已删掉整段「存在但未被任何条目引用」的扫描，本目录连误报的机会都没有。

## 与分类图标的关系

本目录原为「分类图标」预留。分类色板已由设计系统 token 承担
（`FBColor.categoryAccentByName`），T5.5.1 走查后确认**不需要**真实分类图标，
该用途仍处待定状态；日后若启用，命名沿用 `<slug>.webp` 并同样走 `process_images` 管线。
