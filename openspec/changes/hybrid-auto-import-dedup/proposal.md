## Why

`jw` 当前自动导入会把浏览器历史按完整 URL 逐条入库，导致同一域名下的大量相似页面堆积，噪声会稀释跳转质量。现在需要把自动导入从“原始采集”升级为“内置整理”，让自动导入可主导结果，同时保持手动添加的最高优先级。

## What Changes

- 新增 Hybrid 自动导入整理策略：对自动导入记录按 `host + depth-bucket + topic` 聚合，并在“相近深度”范围内仅保留一条代表深页面。
- 调整跳转策略：默认跳转到匹配域/主题下的最近深页面（不是固定首页）。
- 引入来源优先级语义：手动添加记录始终高于自动导入记录；自动导入在非手动冲突场景可主导候选排序。
- 为导入记录补充可排序元信息（来源、聚合键、深度桶、最近命中时间等），以支持稳定、可解释的排序决策。
- 增加整理与回归验证要求，确保降噪后不会显著降低命中率与跳转成功率。

## Capabilities

### New Capabilities
- `history-auto-import-curation`: 定义自动导入记录的聚合、保留、去重与代表页选择规则，降低同域名噪声。
- `jump-resolution-priority`: 定义 `jump` 的来源优先级与默认落点策略（手动优先、默认最近深页面）。

### Modified Capabilities
- None.

## Impact

- Affected code:
  - `cmd/jw/server_runtime.go`（自动导入读取与写入流程）
  - `internal/app/localstore/store.go`（数据模型、写入与排序）
  - `cmd/jw/main.go`（`jump` 行为语义）
  - `README.md`（自动导入与跳转语义文档）
- Data model impact:
  - `~/.jw/store.json` 记录结构将新增来源与聚合相关字段，并需要迁移/兼容策略。
- API/behavior impact:
  - `GET /jump` 的结果在噪声域名场景下会从“任意高频页面”收敛到“最近代表深页面”。
