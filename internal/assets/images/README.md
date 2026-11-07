# assets/images/ —— 图片资源

## 目录约定（扁平结构，2026-09-13 起）

```
images/
├── manifest.json           ← 工具生成，勿手改
├── items/<slug>.webp       ← 食材 / 调料图（1000 张，24.04 MB）
├── categories/<slug>.webp  ← 分类图（36 张，1.00 MB；菜式 tab 三行歌单封面）
└── icons/<slug>.webp       ← 平台 / 自研图标（4 张，由代码常量引用）
```

`<slug>` = 条目名的拼音小写连字符（`番茄` → `fan-qie`、`川菜` → `chuan-cai`）；
多音字 / 需要特指的走 `tool/slug_overrides.json` 手工覆盖（`gen_image_manifest.dart` 读它）。
`categories/` 与 `items/` 是**互相独立的命名空间**：`猪肉`（分类）与 `猪肉`（条目）同 slug
是复用口径（IMG 把同名 items 图拷进 categories/），不是撞名。

**为什么是扁平的**：`pubspec.yaml` 的 assets 声明不递归，旧的「每道菜一个子目录」在 40+ 道菜
规模下要写 40+ 条声明，漏一条图就打包不进 APK。现在 `pubspec.yaml` 固定 4 条声明
（`images/` + `items/` + `categories/` + `icons/`），**加条目不再改 pubspec**。
口径源见 `docs/规范/菜式数据模板.md` §6。

## 菜式封面 + 步骤图已退役（第二批 R2，2026-10-19）

`assets/images/recipes/**`（867 张 B 站 / 小红书截图、20.49 MB）连同关键步骤图目录
**整体退役**，爬取封面的授权台账一并归档；**菜式不再补图**。

- 菜式卡、详情页头图、搜索 / 推荐卡统一走 **色脊 + 分类渐变 + 排版** 降级 —— 这是
  **最终形态，不是临时缺图态**（《第二批 02》§0.3），所以**没有「缺图待补」这回事**；
- 截图来源与作者台账归档在 `docs/archive/素材授权/`（只作历史追溯，不再生成、不随包分发）；
- **门槛**：`tool/validate_assets.dart` 的 **G6** 断言 `recipes/` 目录不存在、目录下无
  `*.webp`、`manifest.json` 顶层无 `recipes` 段 —— 任一违反即 exit 1（门禁日志里有带
  `[G6]` 前缀的结论行）；
- **菜式来源仍逐条署名**：`recipes.json` 的 `source`（platform / author / url）保留不动，
  详情页据此做「看原视频」外跳。退役的是截图素材，不是来源标注 —— 这两件事以前被混在
  一起讲，现在分开：**链接是来源，截图才是素材**。

## 清单是生成的，不要手改

```bash
cd app
dart run tool/gen_image_manifest.dart            # 生成 manifest + 缺图清单
dart run tool/gen_image_manifest.dart --check    # 只比对不写盘（有漂移则退出码 1）
```

一条命令同时产出两份：

- `assets/images/manifest.json` —— 图片解析层（T5.1.2）的运行时清单，每条含 `slug`、图片相对路径、`status`；顶层只有 `items` / `categories` 两段；
- `docs/数据/缺图清单.csv` —— 产图作业单，只有「食材」「分类」两类行（封面 / 步骤行随 R2 一起退役）。

`status` **四值**：

| 值 | 含义 | 判定依据 |
|---|---|---|
| `real` | 本人实拍 | 文件存在，且无来源登记 |
| `stock` | 图库抓取（Pexels / Wikimedia Commons） | 登记在 `app/tool/stock/credits.json` |
| `ai` | 本人用自有出图工具产 | 登记在 `app/tool/ai_credits.json`，或文件名带 `.ai.` |
| `missing` | 无图 | 文件不存在 —— **只有它走 UI 降级** |

（原第五值 `web`（网络截图）随 R2 删除：唯一的来源就是封面截图，素材都没了，留着这个值
只会让「有图」的判定多一条永远为假的岔路。）

**同干双档是个坑**：`<slug>.ai.webp` 与 `<slug>.webp` 在 manifest 里归到同一个逻辑
stem（`_logicalStem` 只剥来源后缀），`_scanImages` 按**真实图优先**定档。所以换图时
**旧档要删**，别指望「新图会自动盖过旧图」——同 stem 并存时进包的是实拍那张，
AI 档留在原地只会在下一次有人把实拍删掉时冒出来。

## 策略

真实图优先 → **食材 / 调料**走图库抓取（Pexels + Commons 双源候选）→ **分类图**沿用
食材图（5 个肉类分类与条目同名拷贝）或图库抓取 → 用户条目允许自拍上传。
出图工具只作兜底，且**新增条目不再登记 `ai` 条目**（T6.7 口径：`ai` 只减不增）。

关键约束：**资源目录可整体替换，版式与图解耦**。
页面里只引用「某个条目对应的图片」，不写死具体文件名，也不在页面里裸调 `Image.asset`。
换图因此**不需要改任何页面代码、不改 DB、也不 bump `kSeedVersion`**。

官方预置图**不落库**：`recipes.json` / `food_items.json` 里没有图片键，DB 的
`image_path` / `cover_image_path` / `photo_paths` 三列存的是「用户自己拍的照片」。
解析优先级（T7.10 起，四档）：**DB 列非空 → 外部图库命中 → 打包 manifest → 渐变降级**。
外部图库只对本机生效、不随包分发，见下方专章。

## 原始素材不入库

手机原图放 `app/tool/inbox/`、图库原图下载放 `app/tool/stock/` —— 两者都在 `.gitignore`
里（3–5MB/张，误提交会撞仓库体积红线）。只有规格化后的成品才落到本目录。

## 外部图库 —— 本机「高清素材层」（T7.10）

把高清实拍 / 图库大图放到**仓库外**的目录（如 `<FoodBox-image-library>`），App 在查打包
manifest **之前**先查这一档。它是**给开发 / 演示用**的能力，只对本机生效、**不随 APK 分发**
—— 打包图仍是兜底。

### 目录与命名约定（中文原名，不用拼音）

```
<root>/
├── 食材/<中文名>.jpg        ← 食材图（也可放 调料/）
└── 调料/<中文名>.png
```

允许扩展名 `.jpg` / `.jpeg` / `.png` / `.webp`，大小写不敏感。文件名用**中文原名**，
与打包档的拼音 slug 互不干扰 —— 两档匹配键不同，`土豆.jpg` 命中外部档、`tu-dou.webp` 命中打包档。
（原来的 `菜式/`、`菜式/步骤/` 两档随 R2 退役，`tool/check_external_gallery.dart` 不再清点它们。）

### 配置（两档位，都不入仓库）

`{ "rootDir": "D:\\FoodBox图库" }` 写进任一位置即可：

1. **开发目录** `app/tool/gallery/local_config.json`（`flutter run` / `flutter test` 的 cwd
   就是 app/，相对路径直接命中；模板见 `local_config.example.json`）；
2. **应用支持目录** `gallery_config.json`（与 `foodbox.sqlite` 同目录，打包后手工放置）。

未配置时本档**整体跳过**，行为与 T7.10 之前完全一致（零回归）。扫描是一次性的
（启动时建表），图库新增 / 改名文件需要重启 App 生效；图库只被查档、不被写。

### 清点工具

```bash
cd app
dart run tool/check_external_gallery.dart                        # 用 local_config.json 的 rootDir
dart run tool/check_external_gallery.dart --root <FoodBox-image-library>   # 或直接指定根目录
```

对照种子数据期望（只剩食材 / 调料一档），输出三张 BOM CSV 到 `docs/数据/`：**已有**
（库里已有且命名正确）、**缺失**（期望有但库里没有，含需要的文件名示例）、**命名不规范**
（位置 / 序号 / 扩展名不合规）。**只读不写图片**；三张 CSV 是本机生成物（已进 `.gitignore`）。

## 图片来源与授权（D9，2026-10-19 按 R2 更新）

按来源分组，逐图明细见对应登记文件（manifest 的 `status` 与之对应）：

| 来源 | 张数 | 授权口径 | 明细登记 |
|---|---|---|---|
| **图库（Pexels / Wikimedia Commons）** | 505（食材 / 调料） + 36（分类图） | Pexels License（免授信、免署名、不得转卖原图）；Commons 逐图不同（PD / CC BY / CC BY-SA，CC-BY 系要求署名） | `app/tool/stock/credits.json`（逐图含 author / license / 图片页 URL） |
| **AI 生成** | 15（均为食材 / 调料） | 本人用出图工具产，无第三方著作权 | 模型：**TRAE 内置 AI 文生图**，生成日期：**2026-09-13**；明细见 `app/tool/ai_credits.json` |
| **菜式封面** | **0**（不随包、不随仓库分发） | 已退役：867 张爬取截图连同授权台账移出素材树 | 归档：`docs/archive/素材授权/` |

**菜式的出处不算图片素材**：`recipes.source` 里的 platform / author / url 是**来源标注**，
随 `recipes.json` 入库并显示在详情页（可外跳原视频）；它不涉及任何素材转载。这是 R2 之后
「授权账」与「来源账」的分界：前者只覆盖随包分发的图，后者覆盖数据里的出处。

## 规格、裁切与隐私

由 `tool/process_images.dart` 强制（口径源，勿凭记忆改）：

| 类型 | 目录 | 长边 | 裁切比例 | 质量 | 单张上限 | 留白 |
|---|---|---|---|---|---|---|
| 食材 / 调料 | `items/` | 512 | 1:1 | q80 | ≤40KB | **0%**（2026-10-09 起不再烘留白，见下） |
| 分类图 | `categories/` | 768 | 3:2 | q65 | ≤80KB | 0% |
| 分类图标 | `icons/` | 512 | 1:1 | q80 | ≤40KB | 8% |

- **留白（`defaultPad`）语义**：值是**单边占画布长边的比例**，`_padResize` 会把内容缩到
  `round(canvas * (1 - 2 * pad))` 居中、四周补 `#F2F2F7`，也就是**烘进像素**、渲染层抠不掉。
  食材图 2026-10-09 起置 0：库页卡片是「实拍图铺满整卡 + 压暗蒙版 + 反白文字」，
  那圈留白在 132×152dp 的卡上就是上下各约 12dp 的浅色横条（肉眼看到的「白条」）。
  存量 520 张用 `dart run tool/dematte_item_images.dart --force` 按同一条几何裁回内容区。
  图标仍留 8%（渲染路径不是铺满卡片，本轮未动）；分类图走铺满档，留白 0。

- **统一裁切口径**：按目标比例**居中裁剪、保持主体居中**（`_centerCropToRatio`），不做任何拉伸；
  裁后长边不足目标尺寸时**不放大**，按原尺寸输出。
- **超上限自动降质**：单张超过上限则每次降 10 质量重编，下限 q35，最多 3 次。
- **隐私 / 元数据**：先用 `bakeOrientation` 依 EXIF 方向套正图片，再经 ffmpeg 有损 WebP
  重编码 —— 重编码顺带**剥离 GPS 等一切元数据**（隐私 + 仓库整洁，实拍原图必带定位，务必走本工具）。

本目录当前约 **25.13 MB**（items 24.04 + categories 1.00 + icons 0.02），
`process_images.dart` 的累计预算仍挂 40MB 不做收紧：R2 退役封面省下 ~20.49 MB 是**包体收益**，
88 条缺图与 AST-02 三批（120/140/132）共 480 条新增图均已于第四批补齐（items 1000 条 0 missing），预算仍有余量。
（历史口径见 `docs/archive/阶段五收尾计划.md` §4.1 D10 / §7.4。）

## 产图流水线（两条路的入口）

| 路 | 原图入口 | 命令（在 `app/` 下） |
|---|---|---|
| **图库抓取**（食材 / 调料 / 分类图） | `tool/stock/<slug>/`（+ `_meta.json` 版权留痕） | `dart run tool/fetch_stock_images.dart --all`（分类图加 `--categories`）；候选须 `dart run tool/build_contact_sheet.dart` 拼宫格后**肉眼筛** |
| **实拍 / 自有出图** | 原图丢进 `app/tool/inbox/`（子目录 `items/` `icons/` `categories/` 可显式指定类型） | 归到下一步统一处理 |
| **统一规格化**（两条路共用） | 读 `inbox/` + 已选 stock 图 | `dart run tool/process_images.dart`（`--dry-run` 预览）→ 居中裁 → 缩放 → ffmpeg 有损 WebP → 写回本目录 → **默认顺带重跑 `gen_image_manifest`** |

无论哪条路，最终**只有规格化后的 WebP 落进 `assets/images/`**；原图 / 图库暂存都在 `.gitignore` 内，不进仓库。
