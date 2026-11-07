# assets/data/ —— 预置数据

数据文件为 JSON，随 `flutter build` 打包进 App。

> **运行时导入**：这些 JSON 不会自动进数据库 —— 由 `lib/data/services/seed_loader.dart`（SeedLoader，W3 新增）在首次启动时读 `rootBundle` 幂等导入 Drift，靠 `SharedPreferences` 的 `seed_version` 标记防重复（不新增 Drift 表）。机制详见《docs/规范/接口约定.md》4.7。
>
> **T7.8（v2.7）起 `food_items.json` 不再走这条导入路**：它是**只读系统库目录**
> —— 由 `lib/data/services/food_catalog.dart`（`FoodCatalog`）在运行时解析成
> `FoodCatalogEntry` 列表，**不落 `items` 表**。用户在录入页「从系统库添加」里
> 勾选之后，那一条才写进库（`is_custom = true`）。理由与口径见《接口约定.md》4.9。

| 文件 | 内容 | 负责方向 | 计划 |
|---|---|---|---|
| `food_items.json` | 食材 / 调料**系统库目录**（88 条，由本人直接提供：名称、分类、默认保质期、默认存放位置、简介四段）。原「≥200 条」批量采集计划已取消，226 条清单降级为**参考 / 扩充池**（见 `docs/数据/整理表填写说明.md`）。**⚠️ 本文件是生成物** —— 由 `docs/数据/食材调料整理表模板.csv` 生成，改数据请改整理表再重新生成，不要手工编辑。**T7.8 起为只读资源、不落库**（见上方提示） | 方向 B | W2 起 |
| `recipes.json` | **种子菜式**（约 8–12 道模拟数据，含步骤、耗时、资源占用、安全标记、适宜月份标签）；后续由**前端菜式编辑器**（FR-REC-11）在使用中扩充，不再批量人工录入。41 道选题清单降级为**素材池**（见 `docs/规范/菜式数据模板.md`） | 方向 C | W3–W4 |
| `alias_map.json` | 食材别名表（西红柿=番茄、马铃薯=土豆 …），供 `RecipeMatcher` 归一。**T1.1 已交付**：`AssetAliasTable.load()` 读平成「别名 → 标准名」反向表，数据已扩至 21 组。**⚠️ 红线**：刻意留缺的名称（`test/data/seed_data_test.dart` 的 `deliberateGaps`）不许收进任何 `aliases`，否则「差一点就能做」会塌成全可做；`recipe_matcher_test.dart` 有用例守住 | 方向 A | T1.1 交付 |
| `glossary.json` | 步骤术语表（焯水 / 爆香 / 收汁 …，F8 期1 范围）。**不落库、不加 schema**：由 `lib/data/services/glossary.dart` 在渲染层匹配 `step.instruction`，命中的词画成可点样式，点开弹解释。关键词（`term` + `aliases`）全局唯一，跨条重名加载即报错；`explain` 一句话讲清「怎么做 / 什么样」，`example` 给具体菜例 | 方向 D | **T5.2.4 已交付** |
| `feed_demo.json` | 动态页演示数据（5–8 条，页面内需标注"演示数据"） | 方向 D | W5 |

## 食材数据的填写方式（W1 起）

`food_items.json` 不再手工编辑 —— 填整理表，两步生成 + 校验：

```bash
cd app
dart run tool/import_food_items.dart   # ① 整理表 → food_items.json（逐行校验，有错不写文件）
dart run tool/validate_assets.dart     # ② 字段完整性关卡，必须全绿
```

- 整理表：`docs/数据/食材调料整理表模板.csv`（UTF-8 BOM，Excel / WPS 双击可直接打开）
- 列口径、11 个分类、覆盖度参考池、保质期参考、简介写法、常见错误：见 `docs/数据/整理表填写说明.md`
- 保存整理表时**必须另存为「CSV UTF-8（逗号分隔）」**，存成普通「CSV」会是 GBK，导入脚本会直接拒收并提示

**校验脚本说明（W2 已就位）**：`tool/validate_assets.dart` 检查字段完整性、枚举合法性、名称去重，以及菜式步骤的**耗时 / 资源占用 / 安全标记**三项质量必填。

## 字段口径（W1 模板约定）

- JSON 键名一律与 Dart 领域模型字段同名（camelCase）；
- 枚举字段填枚举 `name` 字符串，取值见 `lib/data/models/enums.dart` 与《接口约定.md》第 2 章：
  - `kind`：`ingredient` / `seasoning`
  - `storageLocation`：`fridge` / `freezer` / `room`
  - `difficulty`：`easy` / `medium` / `hard`
  - 步骤 `resources`：`stove` / `board` / `hand` / `waiting` 的数组
  - 步骤 `safetyTag`：`none` / `rawMeat` / `readyToEat` / `highHeat`

### `food_items.json`

- 顶层为数组，一个元素一条食材 / 调料；
- 必填：`name`、`category`、`kind`、`shelfLifeDays`、`storageLocation`、简介四段（`introWhat` / `introHow` / `introStore` / `introTip`）；
- 调料可附 `opened`（默认未开封）与 `barcode`（扫码自学习种子数据）；
- 数量、到期日等由用户入库时填写，预置库不写。

### `recipes.json`

- 顶层为数组，一个元素一道菜，`steps` 内嵌步骤数组（导入时拆为 `recipes` + `steps` 两张表）；
- `ingredients` 元素结构与《接口约定.md》2.2 一致；`isRequired` 决定「可做」判定，`isSeasoning` 决定详情页分列；
- **步骤质量三必填**：`durationMinutes`（支持小数）、`resources`、`safetyTag` —— 缺一项时序引擎就无法正确排程；
- `seasonMonths` 填 1–12 的整数数组，是 F12 时令推荐的数据源，每道菜都要标。

### `alias_map.json`

```json
{
  "version": 1,
  "note": "结构说明……",
  "groups": [
    { "canonical": "番茄", "aliases": ["西红柿", "洋柿子"] }
  ]
}
```

- `canonical`：标准名，须与 `food_items.json` 里的 `name` 对齐；
- `aliases`：指向该标准名的其他常见写法；
- **全局唯一**：`canonical` 与所有别名共用一个命名空间，跨组重名会让归一结果不确定（校验脚本会拦下）。
