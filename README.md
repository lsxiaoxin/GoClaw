# GoClaw

GoClaw 是一个使用 Go 和 Eino 构建的编码 Agent。它可以通过本地 CLI 或飞书机器人接收任务，调用工具读取和修改工作区文件，执行受控命令，并在工具权限、Hooks、Todo、子 Agent、Skills、上下文压缩、长期记忆、动态 System Prompt 和错误恢复等环节提供完整的 Agent Harness 能力。

这个项目的重点不是把能力藏在框架里，而是用清晰的 Go 代码实现一个可阅读、可测试、可逐步演进的编码 Agent。

## 功能特性

- Agent Loop：支持模型流式输出、工具调用、工具结果回填、继续推理、最大步数限制、取消任务和错误恢复。
- 工具系统：内置 `bash`、`read_file`、`write_file`、`edit_file`、`glob`、`todo_write`、`task`、`load_skill`、`memory_read` 和 `memory_write`。
- 权限系统：限制工作区边界，区分读写操作，硬拒绝危险命令，并对高风险操作发起人工审批。
- Hooks：支持 `PreToolUse` 和 `PostToolUse`，可按精确工具名或 `*` 通配符匹配，支持超时、阻断和上下文注入。
- Todo 管理：按会话持久化 Todo，支持状态、优先级和 `/status` 汇总。
- 子 Agent：支持隔离上下文的只读子任务，并带有递归深度和并发限制。
- Skills：从 `.goclaw/skills/<name>/SKILL.md` 加载本地技能，并在 Agent 运行前按关键词选择注入。
- 上下文压缩：保存会话历史，并在上下文过长时压缩旧消息，保留最近消息和关键摘要。
- 长期记忆：使用 Markdown 文件存储长期记忆，写入前进行敏感信息检测和人工审批。
- 动态 System Prompt：统一构建包含身份、工作区、安全规则、工具规则、Hooks、Skills、Memory、Todo 和上下文摘要的系统提示。
- 多 Channel：支持本地 CLI、飞书 WebSocket 机器人和测试用 Fake Channel。
- 可读存储：运行状态使用 JSON、JSONL 和 Markdown 文件保存在 `.goclaw/` 下。

## 架构概览

```text
用户 / 飞书 / CLI
        |
        v
internal/channel      统一消息输入输出
        |
        v
internal/app          命令路由和 Agent Run 调度
        |
        v
internal/agent        模型推理循环和工具调用
        |
        v
internal/tool         工具注册与执行
        |
        +--> internal/permission  权限判断和人工审批
        +--> internal/hooks       工具执行前后的扩展点
        +--> internal/store       会话、事件、审批和状态持久化
```

核心目录：

| 路径 | 说明 |
| --- | --- |
| `cmd/goclaw/` | 程序入口和依赖组装 |
| `internal/app/` | 命令路由、任务调度、审批和状态展示 |
| `internal/agent/` | Agent Loop、checkpoint、恢复运行和 Prompt 接入 |
| `internal/tool/` | 工具注册表和内置工具 |
| `internal/permission/` | 权限决策、风险判断和审批策略 |
| `internal/hooks/` | Hook 配置、匹配、执行和注入消息 |
| `internal/channel/` | CLI、Feishu 和 Fake Channel 实现 |
| `internal/store/` | JSON 状态存储、事件去重和审批 checkpoint |
| `internal/todo/` | Todo 模型和会话级持久化 |
| `internal/subagent/` | 子 Agent 请求、结果和执行限制 |
| `internal/skill/` | Skill 加载器和关键词选择器 |
| `internal/contextmgr/` | 会话历史、压缩策略和摘要器 |
| `internal/memory/` | 长期记忆存储和敏感内容检测 |
| `internal/prompt/` | 动态 System Prompt 构建器 |
| `internal/recovery/` | 错误分类和重试策略 |
| `internal/llm/` | OpenAI 兼容 Eino 模型工厂 |

## 环境要求

- Go 1.24 或更高版本。
- 一个 OpenAI 兼容的 Chat Completions 模型服务。
- 可选：如果要接入飞书，需要一个飞书企业自建应用。

## 快速开始

克隆项目并创建配置文件：

```bash
git clone https://github.com/lsxiaoxin/GoClaw.git
cd GoClaw
cp .env.example .env
```

编辑 `.env`，至少填写以下配置：

```env
LLM_API_KEY=your-api-key
LLM_BASE_URL=https://your-openai-compatible-base-url
LLM_MODEL=your-model
```

启动本地 CLI：

```bash
go run ./cmd/goclaw
```

可以输入：

```text
/help
/status
读取 README.md 并总结
查找所有 **/*.go 文件
创建 notes/example.txt，写入 hello，然后读回来
```

写文件、编辑文件、写入长期记忆，以及非明确只读的 Shell 命令都会先进入人工审批流程。

## 配置说明

GoClaw 启动时会读取当前目录下的 `.env` 文件。真实环境变量优先级高于 `.env` 中的同名配置。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `GOCLAW_CHANNEL` | `cli` | 运行通道，可选 `cli` 或 `feishu` |
| `GOCLAW_WORKSPACE` | `.` | Agent 允许操作的工作区 |
| `GOCLAW_DATA_DIR` | `.goclaw` | 运行状态目录 |
| `GOCLAW_LOG_LEVEL` | `info` | 日志级别：`debug`、`info`、`warn`、`error` |
| `GOCLAW_MAX_STEPS` | `8` | 单次 Agent 运行的最大模型循环步数 |
| `GOCLAW_BASH_TIMEOUT` | `10s` | 单次 `bash` 工具执行超时 |
| `GOCLAW_BASH_OUTPUT_LIMIT` | `65536` | `bash` 输出的最大字节数 |
| `GOCLAW_HOOKS_CONFIG` | 空 | 可选 Hook 配置路径 |
| `LLM_API_KEY` | 空 | OpenAI 兼容 API Key |
| `LLM_BASE_URL` | 空 | OpenAI 兼容 Base URL |
| `LLM_MODEL` | 空 | 模型名称 |
| `LLM_TIMEOUT` | `120s` | 单次模型请求超时 |
| `FEISHU_APP_ID` | 空 | `GOCLAW_CHANNEL=feishu` 时必填 |
| `FEISHU_APP_SECRET` | 空 | `GOCLAW_CHANNEL=feishu` 时必填 |
| `FEISHU_ALLOWED_USER_IDS` | 空 | 允许访问机器人的飞书用户 ID，多个用逗号分隔 |
| `FEISHU_ENABLE_GROUPS` | `false` | 是否启用群聊 |
| `FEISHU_ALLOWED_GROUP_IDS` | 空 | 允许访问机器人的飞书群 ID，多个用逗号分隔 |

默认运行状态保存在 `.goclaw/`。该目录包含会话、事件、审批、Todo、上下文历史、长期记忆、Hooks 和 Skills，不应该提交到 Git。

## 使用方式

内置命令：

| 命令 | 说明 |
| --- | --- |
| `/help` | 查看可用命令 |
| `/status` | 查看当前会话、Todo 汇总和最近错误 |
| `/new` | 创建新的会话代次 |
| `/cancel` | 取消当前运行中的任务或待审批任务 |
| `/approve [id]` | 允许一个待审批工具请求 |
| `/deny [id]` | 拒绝一个待审批工具请求 |

常见任务示例：

```text
读取 internal/agent 目录，解释 Agent Loop
找出所有测试文件中覆盖 permission 的用例
新增一个 todo：修复飞书配置文档，priority high
把 notes/design.md 中的 old text 替换成 new text
```

## 飞书机器人接入

GoClaw 使用飞书官方 SDK 的 WebSocket 长连接模式，不需要公网 HTTP 回调地址。

### 1. 创建飞书应用

1. 在飞书开放平台创建“企业自建应用”。
2. 启用“机器人”能力。
3. 记录应用的 `App ID` 和 `App Secret`，写入 `.env`。

```env
GOCLAW_CHANNEL=feishu
FEISHU_APP_ID=cli_xxx
FEISHU_APP_SECRET=xxx
FEISHU_ALLOWED_USER_IDS=ou_xxx
```

### 2. 配置事件订阅

在飞书开放平台中：

1. 打开“事件订阅”。
2. 接收事件方式选择“使用长连接接收事件”。
3. 订阅机器人接收消息相关事件。
4. 如果需要审批卡片按钮，启用卡片交互事件。
5. 发布或安装应用到目标组织。

### 3. 启动机器人

```bash
go run ./cmd/goclaw
```

启动成功后，日志中应出现：

```text
starting GoClaw ... channel=feishu
Feishu channel ready
```

然后在飞书中私聊机器人：

```text
/help
/status
读取 README.md 并总结
```

群聊默认关闭。如果需要启用群聊：

```env
FEISHU_ENABLE_GROUPS=true
FEISHU_ALLOWED_GROUP_IDS=oc_xxx
```

在群聊中必须显式 `@bot`，GoClaw 不会响应 `@所有人`。

## Hooks

GoClaw 默认读取 `.goclaw/hooks.json` 作为 Hook 配置。也可以通过 `GOCLAW_HOOKS_CONFIG` 指定其他路径。

示例：

```json
{
  "hooks": [
    {
      "event": "PreToolUse",
      "matcher": "bash",
      "builtin": "inject",
      "timeout": "500ms",
      "message": "About to run {{tool}} with {{arguments}}"
    },
    {
      "event": "PostToolUse",
      "matcher": "*",
      "builtin": "record",
      "timeout": "500ms",
      "message": "{{tool}} finished in {{elapsed}}"
    }
  ]
}
```

当前支持的内置 Hook 动作为 `allow`、`block`、`inject` 和 `record`。外部 `command` Hook 会被解析并保留接口，但默认不会执行不可信脚本。

## Skills

Skill 放在 `.goclaw/skills/<name>/SKILL.md`。

最小示例：

```markdown
---
name: go-review
description: Review Go code for correctness, tests, and maintainability.
keywords:
  - go
  - review
---

Check error handling, table-driven tests, data races, and package boundaries.
```

GoClaw 会在 Agent 运行前按关键词选择相关 Skill，并把选中的 Skill 元数据注入 Prompt。完整 Skill 正文可通过 `load_skill` 工具读取。

## Memory

长期记忆以 Markdown 文件形式保存在 `.goclaw/memory/`。GoClaw 会在 Agent 运行前选择相关记忆注入上下文，也可以通过 `memory_write` 写入新记忆。

`memory_write` 必须经过人工审批，并会拒绝疑似敏感信息，例如 API key、token、password、secret、精确地址和身份证号。

## 安全模型

GoClaw 的默认安全边界是配置中的工作区。

- 文件工具会拒绝绝对路径、父目录逃逸和符号链接逃逸。
- 危险 Shell 命令会被硬拒绝。
- 高风险写操作需要人工审批。
- Hooks 在权限检查之后执行，不能绕过权限结果。
- Skill 和 Memory 不能覆盖安全规则、工作区规则、权限规则或 Hook 规则。
- 工具失败、权限拒绝、Hook 阻断、上下文压缩失败和模型错误都会以清晰结果返回给 Agent 或显示在 `/status` 中。

## 开发与测试

运行全量测试：

```bash
go test ./...
```

常用检查：

```bash
go test -race ./...
go vet ./...
gofmt -w ./cmd ./internal
```

默认测试完全离线，使用 Fake Model、Fake Channel、临时工作区和确定性测试数据，不依赖真实模型 API Key 或飞书凭据。

## 文档

- [实现计划](doc/plan.md)
- [阶段代码导读](doc/chapters/)

已完成阶段：

| 阶段 | 主题 | Tag |
| --- | --- | --- |
| s00 | 工程启动 | `s00-bootstrap` |
| s01 | Agent Loop | `s01-agent-loop` |
| s02 | Tool Use | `s02-tool-use` |
| s03 | Permission | `s03-permission` |
| s04 | Hooks | `s04-hooks` |
| s05 | TodoWrite | `s05-todo-write` |
| s06 | Subagent | `s06-subagent` |
| s07 | Skill Loading | `s07-skill-loading` |
| s08 | Context Compact | `s08-context-compact` |
| s09 | Memory | `s09-memory` |
| s10 | System Prompt | `s10-system-prompt` |
| s11 | Error Recovery | `s11-error-recovery` |

## 常见问题

启动时报 `LLM_API_KEY` 或 `LLM_MODEL` 缺失：

- 确认已执行 `cp .env.example .env`。
- 填写 `LLM_API_KEY`、`LLM_BASE_URL` 和 `LLM_MODEL`。

飞书启动成功但机器人不回复：

- 确认 `GOCLAW_CHANNEL=feishu`。
- 确认应用已经发布或安装到当前组织。
- 确认事件订阅使用 WebSocket 长连接。
- 确认 `FEISHU_ALLOWED_USER_IDS` 包含发送人的 `ou_xxx` 用户 ID。
- 查看日志中是否出现 `Feishu message rejected`。

群聊不回复：

- 设置 `FEISHU_ENABLE_GROUPS=true`。
- 把群 ID `oc_xxx` 加入 `FEISHU_ALLOWED_GROUP_IDS`。
- 在群里显式 `@bot`。

审批按钮无反应：

- 确认飞书应用已启用卡片交互事件。
- 使用文本命令兜底：`/approve <id>` 或 `/deny <id>`。

## License

当前仓库尚未包含 License 文件。如果要作为开源项目公开分发，建议先补充明确的开源许可证。
