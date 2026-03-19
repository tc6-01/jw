# jw

终端网页快速跳转工具（zoxide-like for web）。

你可以把常用网页记到本地，然后直接用关键词跳转。

## 30 秒上手
```bash
jw tutorial
jw add https://github.com GitHub
jw github
jw list
```

## 产品介绍与命令入口
- `jw about`：产品介绍与 30 秒上手路径（短平快）。
- `jw help`：完整命令入口与语义说明（权威速查）。
- `jw tutorial`：可执行教程，按步骤跑完整流程。

## 命令速查（精简）
- `jw server`：启动本地记录服务（自动选择空闲端口），提供 `/health`、`/record`、`/jump` 接口。
- `jw add <url> [title]`：向本地库写入或更新网址记录。
- `jw query <keyword>`：查看关键词候选与分数。
- `jw jump <keyword>` / `jw <keyword>`：打开最佳匹配网页，并更新命中计数。
- `jw list` / `jw rm <url|title>`：查看和清理本地记录。
- 需要完整说明时，请运行 `jw help`。

## 本地记录服务
启动：
```bash
jw server
```

服务启动后会打印地址，例如：`http://127.0.0.1:18888`。

接口：
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

## 数据存储
- 本地文件：`~/.jw/store.json`
- 已做 URL 规范化与敏感参数脱敏

## 安装
推荐使用 Homebrew：
```bash
brew tap tc6-01/homebrew-tap
brew install tc6-01/tap/jw
```

## 常见问题
- 没有匹配结果怎么办？
  - 先用 `jw query <keyword>` 看候选，再调整关键词或先 `jw add`。
- 本地数据存在哪里？
  - 默认在 `~/.jw/store.json`。
- `jw server` 端口冲突怎么办？
  - 默认会自动选择空闲端口并打印地址。

## 可视化界面计划
- 当前版本聚焦 CLI 体验。
- Web 可视化界面会作为后续迭代方向（当前未实现）。

## 许可证
MIT
