# jw

终端网页快速跳转工具（zoxide-like for web）。

用关键词快速跳转常用网页，并支持本地服务与自动导入浏览器历史。

## 快速上手

### 1) 安装

```bash
brew tap tc6-01/homebrew-tap
brew install tc6-01/tap/jw
```

### 2) 30 秒体验

```bash
jw add https://github.com GitHub
jw add https://docs.github.com GitHubDocs
jw query github
jw github
```

### 3) 启用后台服务（可选）

```bash
jw server start
jw server status
```

### 4) 启用自动导入浏览器历史（可选）

```bash
jw config auto-import-history on
```

## 特性说明（完整）

- 关键词跳转：`jw <keyword>` / `jw jump <keyword>`
- 候选查询：`jw query <keyword>`
- 手动记录：`jw add <url> [title]`
- 记录管理：`jw list` / `jw rm <url|title>`
- 本地 HTTP 服务：`jw server [addr]`、`jw server start|stop|status`
- 自动导入历史：从 Chrome History 定时增量导入（服务运行时）
- 自动导入内置整理（Hybrid）：按 `host + depth-bucket + topic` 分组
- 相近深度去重：同分组只保留 1 个自动导入代表页
- 不同深度保留：不同深度分桶互不覆盖
- 跳转优先级：`manual > legacy > auto`
- 默认落点策略：优先最近深页面（CLI 与 `/jump` 共用同一解析逻辑）
- 安全与隐私：危险协议拦截、URL 规范化、敏感参数脱敏
- 数据本地化：默认仅本地文件，不依赖云端

## 命令速查

- `jw help`：完整命令入口
- `jw tutorial`：可执行教程
- `jw about`：产品介绍与上手路径
- `jw add <url> [title]`：手动添加/强化记录
- `jw query <keyword>`：查看候选及分数
- `jw jump <keyword>` / `jw <keyword>`：跳转最佳匹配
- `jw list`：查看记录
- `jw rm <url|title>`：删除记录
- `jw server [addr]`：前台运行本地服务
- `jw server start [addr]`：后台运行本地服务
- `jw server stop`：停止后台服务
- `jw server status`：查看后台服务状态
- `jw config`：查看配置
- `jw config auto-import-history on|off`：开启/关闭自动导入

## 服务接口

- `GET /health`
- `POST /record`
- `GET /jump?q=<keyword>`

`POST /record` 请求体示例：

```json
{
  "url": "https://github.com",
  "title": "GitHub"
}
```

## 实现原理（简版）

- 存储：本地 `~/.jw/store.json`
- 写入来源：
  - `manual`：用户主动添加/强化（最高优先级）
  - `auto`：浏览器历史自动导入
  - `legacy`：旧记录兼容语义（中性优先级）
- 自动导入流程：
  - 读取 Chrome History
  - URL 规范化与脱敏
  - 生成分组键（`host + depth-bucket + topic`）
  - 分组内仅保留 1 个 auto 代表页
- 排序：关键词匹配 + 频次 + 时间衰减 + 来源权重
- 跳转解析：CLI `jw jump` 与 HTTP `GET /jump` 使用同一解析函数

## FAQ

- 没有匹配结果怎么办？
  - 先执行 `jw query <keyword>` 查看候选，再优化关键词或先 `jw add`。

- 自动导入为什么没有生效？
  - 需要同时满足：
  - 后台服务正在运行（`jw server start`）
  - 自动导入已开启（`jw config auto-import-history on`）
  - 系统可读取 Chrome History（且本机有 `sqlite3`）

- 我执行 `jw server status` 出现 `address status: missing port in address`？
  - 这是旧版二进制行为。升级到新版 `jw` 后即可使用 `server start|stop|status` 与 `config` 命令。

- 本地数据在哪里？
  - 记录文件：`~/.jw/store.json`
  - 配置文件：`~/.jw/config.json`
  - 后台状态：`~/.jw/server-state.json`
  - 后台日志：`~/.jw/server.log`

- 端口冲突怎么办？
  - 默认自动选择空闲端口，也可以显式指定地址：`jw server 127.0.0.1:18888`。

## 版本升级计划

- 当前基线：`v0.1.x`（CLI 基础能力 + 本地 server）
- `v0.2.x` 目标：
  - 自动导入整理策略稳定化（Hybrid 规则持续调优）
  - 跳转优先级策略稳定化（manual/legacy/auto）
  - 增强回归测试与导入可观测性
- `v0.3.x` 目标：
  - 配置能力增强（更细粒度导入整理策略开关）
  - 数据规模下的性能优化（查询与导入路径）
  - 跨平台兼容性补强（macOS/Linux 细节打磨）
- `v1.0.0` 目标：
  - 能力边界稳定，默认配置可覆盖主要使用场景
  - 数据迁移路径稳定，升级成本可控
  - 文档、测试、发布流程达到长期维护标准

升级兼容策略：
- 小版本（`v0.x.y`）优先保持命令兼容与默认行为稳定。
- 涉及数据结构变化时，提供自动兼容迁移（如 `legacy` 语义）并在 README 明确说明。
- 若出现行为回退风险，优先通过配置开关提供临时回滚路径。

## 许可证

MIT
