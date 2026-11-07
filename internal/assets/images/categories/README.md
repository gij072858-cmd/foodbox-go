# categories/ —— 分类图命名空间（v4b 第 2 项）

菜式 tab 三行歌单的封面图，每类一张，共 36 张（主料 14 / 菜系 18 / 做法 4）。
slug = 分类名拼音小写连字符（`川菜` → `chuan-cai`），名单与映射由
`tool/image_manifest_categories.dart` 生成、`tool/gen_image_manifest.dart` 收进
manifest 顶层 `categories` 段；关键词与产图作业单见 `docs/数据/缺图清单.csv` 的分类行。
5 个复用主料（猪肉/牛肉/羊肉/鸡肉/鸭肉）= items 同名图拷贝。

图片入库前本目录只有本 README 占位（flutter 要求已声明的 asset 目录必须存在）。
