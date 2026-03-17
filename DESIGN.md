# jw 设计文档（zoxide-like 网页跳转工具）

## 1. 目标与范围

### 1.1 目标
- 在终端中提供 `查询 -> 命中 -> 跳转` 的最短路径。
- 用本地行为数据持续优化排序效果（frecency + context）。
- 默认隐私保护，离线可用，跨平台支持 macOS/Linux。

### 1.2 非目标
- 不做网页自动化（点击、表单、脚本录制）。
- 不做云端同步与多人协作。
- 不做复杂语义检索或模型推荐系统。

## 2. 交互模型

### 2.1 命令集合（MVP）
- `jw add <url>`：手动添加或强化一个目标 URL。
- `jw query <keyword>`：返回候选列表（支持 `--json`、`--explain`）。
- `jw jump <keyword>`：命中第一候选并直接打开。
- `jw list`：查看当前已学习的目标。
- `jw rm <url|id>`：删除目标。

### 2.2 输入输出边界
- 输入：查询词（必选），可选上下文（cwd/workspace）。
- 输出：候选（title/url/score/reason）与跳转结果（success/fail）。
- 错误：空输入、非法 URL、危险协议统一错误码与文案。

## 3. 系统架构

### 3.1 分层
- CLI 层：参数解析、子命令路由、格式化输出。
- 应用层：编排查询、跳转、索引更新流程。
- 领域层：排序、规则、URL 规范化、隐私策略。
- 基础设施层：SQLite、浏览器打开适配、配置与日志。

依赖方向：`CLI -> 应用层 -> 领域接口 -> 基础设施实现`。

### 3.2 模块
- `cmd/jw`：程序入口。
- `internal/domain/urlnorm`：URL 规范化与脱敏。
- `internal/domain/ranker`：评分函数与参数。
- 后续扩展：`internal/app`、`internal/infra/{db,opener,config}`。

## 4. 排序算法

### 4.1 总体公式
`S = B * W_ctx`

- `B = Σ(w_alias * exp(-lambda * delta_t))`
- `W_ctx = w_dir * w_kw * w_tod * w_alias_boost`

### 4.2 默认参数
- 半衰期 `H = 14 天`，`lambda = ln(2) / H`。
- `beta_dir = 0.35`。
- `beta_kw = 0.9`。
- 上下文总增益上限 `W_ctx_max = 2.5`。

### 4.3 抗噪机制
- 同 URL 10 秒去抖。
- 异常高频点击降权。
- 冷启动先验 `epsilon = 0.05`。

## 5. 数据模型（SQLite）

### 5.1 表结构（MVP + 可扩展）
- `schema_meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)`
- `destinations(id INTEGER PRIMARY KEY, url TEXT UNIQUE NOT NULL, title TEXT, first_seen_at INTEGER, last_seen_at INTEGER, total_visits INTEGER DEFAULT 0, pinned INTEGER DEFAULT 0, hidden INTEGER DEFAULT 0, metadata_json TEXT DEFAULT '{}')`
- `visit_events(id INTEGER PRIMARY KEY, destination_id INTEGER NOT NULL, visited_at INTEGER NOT NULL, alias TEXT NOT NULL, cwd_hash TEXT, keyword TEXT, time_bucket INTEGER NOT NULL)`
- `destination_stats(destination_id INTEGER PRIMARY KEY, score_base REAL NOT NULL DEFAULT 0, score_updated_at INTEGER NOT NULL, visits_7d INTEGER DEFAULT 0, visits_30d INTEGER DEFAULT 0)`
- `context_stats(id INTEGER PRIMARY KEY, destination_id INTEGER NOT NULL, ctx_type TEXT NOT NULL, ctx_key TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 0, last_seen_at INTEGER NOT NULL, UNIQUE(destination_id, ctx_type, ctx_key))`

### 5.2 迁移策略
- 仅前向迁移：Vn -> Vn+1。
- 每次迁移事务化，支持幂等。
- 若本地 DB 版本高于客户端，进入只读降级模式。

## 6. 安全与隐私

### 6.1 默认安全基线
- 开启敏感参数脱敏。
- 拦截危险协议：`file`、`javascript`、`data`、`vbscript`、`about`、浏览器内部协议。
- 历史默认保留 30 天，审计默认 90 天。
- 本地加密优先系统密钥链。

### 6.2 脱敏规则
默认敏感 key：
`token, access_token, refresh_token, id_token, code, state, session, sid, password, passwd, secret, api_key, key, auth, jwt, signature, sig`

## 7. 里程碑

### 7.1 MVP
- 完成 `add/query/jump/list/rm`。
- 完成 URL 规范化与隐私脱敏。
- 完成基础评分（时间衰减 + 上下文）与单测。

### 7.2 Beta
- 增强多因子排序与 explain。
- 增加兼容层与结构化观测。

### 7.3 GA
- 完成 SLO、告警、Runbook、回滚演练。

## 8. 验收标准（MVP）
- 常见场景跳转成功率 >= 95%。
- 查询延迟 P50 < 120ms，P95 < 300ms。
- 默认配置下不落盘敏感参数明文。
