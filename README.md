# jw

`jw` 是一个给常用网页做“地址栏快捷跳转”的命令行工具。

你可以把它理解成：

- 网页版 `zoxide`
- 浏览器地址栏增强器
- 给常去站点、文档页、后台页做“短命令直达”的工具

它会记住你常访问的网址，让你在终端里用更短的输入直接打开目标页面。比如输入 `jw github/docs`，就能直接跳到对应文档，而不是每次都从书签、历史记录或搜索结果里重新找。

## 它适合什么场景

如果你经常遇到这些情况，`jw` 会很顺手：

- 经常在 GitHub、文档站、内部后台、CI 页面之间来回切换
- 记得站点大概结构，但不想每次都手动点进多级路径
- 书签太多，浏览器历史太乱，真正常用的页面反而难找
- 想把“打开某个网页”变成和 `z foo` 一样快的肌肉记忆

典型例子：

- `jw github`
- `jw github/docs`
- `jw github/docs/actions`
- `jw figma`
- `jw jira`

## jw 解决的核心问题

浏览器已经有书签、历史记录和地址栏补全，但它们都有同一个问题：

- 输入成本高
- 命中不稳定
- 常用页面和噪音页面混在一起
- 很难形成统一、可复用的“短路径”习惯

`jw` 的思路是把常用网页整理成一个可学习的 host/path 导航树：

- `github` 表示站点入口
- `github/docs` 表示这个站点下的常用分区
- `github/docs/actions` 表示更具体的页面

这样你不需要记完整 URL，只需要记住“像地址栏一样”的短路径。

## 30 秒上手

### 1) 安装

推荐通过 Homebrew 安装：

```bash
brew tap tc6-01/homebrew-tap
brew install tc6-01/homebrew-tap/jw
```

如果你在开发或想直接从源码安装，也可以：

```bash
go install github.com/tc6-01/jw/cmd/jw@latest
```

安装后可以先确认版本：

```bash
jw version
```

### 2) 先手动记几个常用页面

```bash
jw add https://github.com GitHub
jw add https://github.com/docs/actions GitHubActions
```

### 3) 看看它会怎么理解你的输入

```bash
jw query github/docs
```

### 4) 直接跳转

```bash
jw github
jw github/docs/actions
```

如果你平时就常开固定网页，这几步已经够开始用了。

## 它是怎么工作的

`jw` 主要有两种“喂数据”的方式。

### 1) 手动添加

适合你明确知道哪些页面值得保留。

```bash
jw add <url> [title]
```

这是最直接、最可控的方式。你手动加入的页面会成为 `jw` 学习的重要依据。

### 2) 自动导入浏览器历史

适合让 `jw` 从你真实使用过的网页里慢慢学习。

```bash
jw config auto-import-history on
```

开启后，`jw` 会在后台服务运行时定时从 Chrome History 增量导入记录，把高频访问页面整理进导航树。

如果你不想依赖自动导入，也完全可以只用手动添加。

## “服务”和“自动导入”分别是什么

这是第一次接触 `jw` 最容易混淆的地方。

### 服务是什么

`jw server` 是一个本地 HTTP 服务。

它的作用不是把数据放到云端，而是让 `jw` 在本机后台持续运行，处理这些能力：

- 提供本地跳转/记录接口
- 维持自动导入浏览器历史的周期任务
- 让浏览器或其他本地工具更方便接入

常用命令：

```bash
jw server start
jw server status
jw server stop
```

### 自动导入是什么

自动导入是一个可选能力，不是必须项。

它会读取本机 Chrome 历史记录，把你真实访问过的网址转成更适合跳转的结构。它依赖后台服务运行，但和“跳转能力”不是一回事。

简单理解：

- 想要最轻量使用：只用 `jw add` + `jw <keyword>`
- 想让它自动成长：再开启 `server` + `auto-import-history`

## 核心能力

- 关键词跳转：`jw <keyword>` 或 `jw jump <keyword>`
- 地址式跳转：`jw github/docs` 这种输入会优先按 host/path 理解
- 候选查看：`jw query <keyword>` 先看命中结果，再决定跳哪里
- 手动记忆：`jw add <url> [title]`
- 基本管理：`jw list`、`jw rm <url|title>`
- 本地服务：`jw server start|stop|status`
- 自动导入：从 Chrome 历史增量学习常用网页
- 本地数据：默认存储在本机，不依赖云端

## 一个更直观的理解方式

你可以把 `jw` 想成给网页建立了一套“快捷路径系统”：

- `github` 不只是一个站点名，而是一个入口
- `github/docs` 不只是搜索词，而是一个常用路径节点
- 当你输入一个上层节点时，`jw` 会学习并跳到它更合适的默认目标

这也是它和普通书签的区别：

- 书签更像静态收藏夹
- `jw` 更像可学习、可导航的网页路径入口

## 命令速查

- `jw help`：查看完整命令
- `jw tutorial`：运行可执行教程
- `jw about`：查看产品说明
- `jw add <url> [title]`：手动添加页面
- `jw query <keyword>`：查看候选结果
- `jw jump <keyword>` / `jw <keyword>`：直接跳转
- `jw list`：查看记录
- `jw rm <url|title>`：删除记录
- `jw server [addr]`：前台运行本地服务
- `jw server start [addr]`：后台启动本地服务
- `jw server stop`：停止后台服务
- `jw server status`：查看后台服务状态
- `jw config`：查看配置
- `jw config auto-import-history on|off`：开启/关闭自动导入

## 当前限制

当前版本更适合“个人、本机、Chrome 为主”的使用方式，已知边界包括：

- 自动导入目前面向 Chrome History
- 自动导入依赖后台服务运行
- 某些更细粒度规则仍偏底层，主要通过本地数据文件配置
- 更适合个人知识型/工作型跳转，不是团队书签同步系统

如果你只是想要一个本地、快速、可学习的网页跳转器，这些限制通常不影响日常使用。

## FAQ

- 没有匹配结果怎么办？
  - 先用 `jw query <keyword>` 看候选；如果结果不理想，直接 `jw add` 你真正想去的页面。

- 自动导入为什么没生效？
  - 通常需要同时满足两件事：后台服务正在运行，以及 `jw config auto-import-history on` 已开启。

- 本地数据放在哪里？
  - 记录文件：`~/.jw/store.json`
  - 配置文件：`~/.jw/config.json`
  - 服务状态：`~/.jw/server-state.json`
  - 服务日志：`~/.jw/server.log`

- 端口冲突怎么办？
  - 可以显式指定地址，例如 `jw server 127.0.0.1:18888`。

- 我必须开启后台服务吗？
  - 不必须。只做手动添加和跳转时，可以不启用。只有自动导入和本地服务接口依赖 `jw server`。

## 发布与 Homebrew

当前仓库使用 GitHub tag 驱动发布：

- 推送 `v*` tag 后，GitHub Actions 会执行 GoReleaser
- GoReleaser 会构建 release 产物并创建 GitHub Release
- 同时自动更新 `tc6-01/homebrew-tap` 中的 `jw` formula

如果你是用户，通常只需要执行：

```bash
brew update
brew upgrade tc6-01/homebrew-tap/jw
```

注意：自动导入浏览器历史依赖系统里的 `sqlite3` 可执行文件；即使通过 release 或 Homebrew 安装成功，这一能力也仍需要本机具备该依赖。

## 本地服务接口

如果你需要把 `jw` 接到浏览器脚本或其他本地工具上，可以使用这些接口：

- `GET /health`
- `POST /record`
- `GET /jump?q=<keyword>`

`POST /record` 示例：

```json
{
  "url": "https://github.com",
  "title": "GitHub"
}
```

## 许可证

MIT
