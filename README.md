# jw

终端网页快速跳转工具（zoxide-like for web）。

你可以把常用网页记到本地，然后直接用关键词跳转。

## 核心能力
- `jw server`：启动本地记录服务（自动选择空闲端口）
- `jw add <url> [title]`：手动添加常用网页
- `jw query <keyword>`：查看候选结果
- `jw jump <keyword>`：跳转最佳匹配
- `jw <keyword>`：快速跳转（等价于 `jw jump <keyword>`）
- `jw list`：查看本地记录
- `jw rm <url|title>`：删除记录
- `jw tutorial`：运行内置可执行教程

## 快速开始
```bash
jw tutorial
```

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
推荐使用 Homebrew（发布后可用）：
```bash
brew tap <your-tap>
brew install jw
```

## 许可证
MIT
