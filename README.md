# GoClaw

GoClaw 是一个使用 Go 和 Eino 构建的学习型编码 Agent。目标是从最小 Agent
Loop 开始，逐步实现工具、权限、Hooks、Todo、子 Agent、Skills、上下文压缩、
记忆、动态 System Prompt 和错误恢复，并通过飞书等 IM 操作本地工作区。

当前阶段：`s11-error-recovery`

> s11 已加入错误分类、模型有限重试和最近错误摘要。GoClaw 会区分 model、tool、
> permission、hook、compact、store 和 config 错误，并在 `/status` 展示最近错误。

## 学习方式

- 完整路线保存在 [doc/plan.md](doc/plan.md)。
- 每章说明保存在 `doc/chapters/`。
- 每章由 2 至 4 个小提交组成。
- 每章完成后创建 Git 标签，可使用 `git show s00-bootstrap` 或
  `git diff s00-bootstrap..s01-agent-loop` 阅读演进。
- 每章完成后暂停，先阅读代码和提交历史，再进入下一章。

参考项目：

- <https://github.com/shareAI-lab/learn-claude-code>
- GoClaw 参考其根目录新版教程的前 11 章，但使用 Go、Eino 和 IM 场景重新设计。

## 已实现

- 环境变量配置和校验。
- OpenAI 兼容 Eino `AgenticModel` 工厂。
- 可直接阅读的 JSON 文件状态存储。
- Event ID 跨进程持久化去重。
- CLI、飞书 WebSocket 长连接和 Fake Channel。
- 流式回复抽象。
- `/help`、`/status`、`/new`、`/cancel`、`/approve`、`/deny`。
- 系统信号驱动的安全退出。
- 模型调用、工具调用、工具结果回填和继续推理的 Agent Loop。
- 模型文本通过 `Channel.Stream` 增量返回。
- 相同 Chat ID 串行运行，不同 Chat ID 可并发。
- `/cancel` 可取消模型请求和正在执行的 `bash`。
- `bash` 在工作区执行，并带有超时、输出上限和硬拒绝规则。
- 最大 step 上限，避免无终止条件的循环。
- `ToolRegistry` 统一管理工具 Schema、严格参数校验和执行分发。
- `read_file`、`write_file`、`edit_file` 和 `glob`。
- 文件工具拒绝绝对路径、父目录逃逸和符号链接逃逸。
- `edit_file` 只替换唯一的精确匹配，避免模糊修改。
- 连续只读工具并发执行，写工具顺序执行，工具结果保持模型调用顺序。
- 权限结果统一为 `allow`、`ask`、`deny` 和 `invalid`。
- 危险系统命令走硬拒绝，不会进入人工审批。
- `read_file`、`glob` 和明确只读的 bash 命令自动执行。
- `write_file`、`edit_file` 和非明确只读 bash 命令必须审批。
- `/approve [ID]`、`/deny [ID]`，以及 `/cancel` 取消待审批任务。
- 飞书优先发送带允许/拒绝按钮的审批卡片，文本命令作为后备。
- 审批 checkpoint 保存在 `.goclaw/approvals/`，等待期间重启后仍可继续。
- 审批只能由原任务发起人处理。
- `PreToolUse` 和 `PostToolUse` Hook。
- Hook 支持精确工具名和 `*` 通配匹配。
- `PreToolUse` 可放行、阻断或注入提示；阻断时真实工具不会执行。
- `PostToolUse` 可观察工具结果并注入提示。
- Hook 执行有超时控制，错误和 panic 不会导致程序崩溃。
- Hook 注入内容会进入后续模型上下文。
- Hook 在权限系统之后运行，不能绕过硬拒绝、非法参数或人工审批。
- `todo_write` 按 chat/session 持久化任务列表。
- Todo 支持 `pending`、`in_progress`、`completed` 和 `low/medium/high` 优先级。
- 同一会话最多一个 Todo 处于 `in_progress`。
- Todo 数据保存到 `.goclaw/todos/`，不同会话互相隔离。
- `/status` 展示 Todo 总数和各状态计数。
- 连续三轮未更新 Todo 时，Agent 会向后续模型上下文注入提醒。
- `task` 工具可创建同步子 Agent 并返回结果摘要。
- 子 Agent 默认只使用 `read_file` 和 `glob`，不污染父 Agent 上下文。
- 子 Agent 工具调用仍经过权限和 Hook 管线。
- 子 Agent 有最大递归深度和最大并发限制。
- 从 `.goclaw/skills/<name>/SKILL.md` 加载本地 skill。
- Agent Run 前用关键词选择相关 skill，并只注入名称和描述。
- `load_skill` 工具可按名称返回完整 skill instructions。
- Skill 不能覆盖安全、权限、Hooks 或 workspace 规则。
- Agent Run 前会加载 `.goclaw/context/` 中的会话历史。
- 超过压缩策略阈值时，旧消息会被压缩为 summary，最近消息原样保留。
- summary 会保留工具结果、关键错误和 Todo 状态摘要等运行信号。
- 上下文压缩失败不会让 Agent 崩溃，会保存未压缩历史并记录日志。
- `.goclaw/memory/` 支持 `user`、`feedback`、`project`、`reference` 四类 Markdown 记忆。
- Agent Run 前按关键词选择相关 memory，并注入长期记忆上下文。
- `memory_read` 可读取相关记忆，`memory_write` 可写入长期记忆。
- `memory_write` 必须经过人工审批，并拒绝 API key、token、password、secret、精确地址和身份证号等敏感内容。
- `internal/prompt` 统一构建动态 System Prompt。
- Prompt 明确危险工具需要权限、不能泄露 secret、不能越过 workspace。
- Skills、Memory、Todo 和上下文 Summary 可按模块启用或禁用，不能覆盖安全规则。
- `internal/recovery` 提供错误分类和 RetryPolicy。
- 模型失败会有限重试，超过上限后返回清晰 `ModelError`。
- 工具失败、权限拒绝和 Hook 阻断不会自动重试危险操作。
- `/status` 展示最近错误摘要，便于用户理解恢复状态。

## 快速开始

环境要求：

- Go 1.24 或更高版本。
- 一个 OpenAI 兼容模型服务。
- 如需飞书接入，需要飞书开放平台企业自建应用。

### 1. 运行本地 CLI

```bash
cp .env.example .env
# 编辑 .env，填写 LLM_API_KEY、LLM_BASE_URL 和 LLM_MODEL
go run ./cmd/goclaw
```

输入：

```text
/help
/status
/new
/approve
/deny
hello
```

也可以尝试：

```text
读取 README.md 并总结
创建 notes/example.txt，写入 hello，然后读回来
查找所有 **/*.go 文件
```

### 2. 运行飞书机器人

飞书模式使用官方 SDK 的 WebSocket 长连接，不需要公网 HTTP 回调地址。先完成下方
“飞书接入”配置，然后把 `.env` 改为：

```env
GOCLAW_CHANNEL=feishu
GOCLAW_WORKSPACE=/absolute/path/to/your/workspace
GOCLAW_DATA_DIR=.goclaw

LLM_API_KEY=your-api-key
LLM_BASE_URL=https://your-openai-compatible-base-url
LLM_MODEL=your-model

FEISHU_APP_ID=cli_xxx
FEISHU_APP_SECRET=your-app-secret
FEISHU_ALLOWED_USER_IDS=ou_xxx
```

启动：

```bash
go run ./cmd/goclaw
```

启动成功时日志会包含：

```text
starting GoClaw ... stage=s11-error-recovery channel=feishu
Feishu channel ready
```

然后在飞书中私聊机器人发送：

```text
/help
/status
读取 README.md 并总结
```

写文件、编辑文件或执行非明确只读的 bash 时，GoClaw 会先要求人工审批。
`todo_write` 作为普通工具执行，也会经过权限和 Hook 管线。
`task` 默认用于只读调查，父 Agent 只会看到子 Agent 的摘要。
`load_skill` 用于读取已配置 skill 的完整说明。
上下文历史保存在 `.goclaw/context/`，不同会话互相隔离。
长期记忆保存在 `.goclaw/memory/`，写入前需要人工审批。

运行状态默认保存在当前工作区的 `.goclaw/`，该目录不会提交到 Git。

## 配置

复制环境变量模板并按需填写：

```bash
cp .env.example .env
```

GoClaw 启动时自动读取当前目录的 `.env`。系统环境变量优先于 `.env` 中的同名配置。

主要变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `GOCLAW_CHANNEL` | `cli` | `cli` 或 `feishu` |
| `GOCLAW_WORKSPACE` | `.` | Agent 允许操作的工作区 |
| `GOCLAW_DATA_DIR` | `.goclaw` | 运行状态目录 |
| `GOCLAW_LOG_LEVEL` | `info` | `debug/info/warn/error` |
| `GOCLAW_MAX_STEPS` | `8` | 单次 Agent 运行的最大模型调用步数 |
| `GOCLAW_BASH_TIMEOUT` | `10s` | 单次 bash 命令超时 |
| `GOCLAW_BASH_OUTPUT_LIMIT` | `65536` | bash 合并输出的最大字节数 |
| `GOCLAW_HOOKS_CONFIG` | 空 | 可选 Hook JSON 配置路径；默认读取工作区 `.goclaw/hooks.json` |
| `LLM_API_KEY` | 空 | OpenAI 兼容 API Key，启动时必填 |
| `LLM_BASE_URL` | 空 | OpenAI 兼容 API Base URL |
| `LLM_MODEL` | 空 | 模型名称，启动时必填 |
| `LLM_TIMEOUT` | `120s` | 单次模型请求超时 |

Hook 配置示例：

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

当前默认只执行安全内置 Hook：`allow`、`block`、`inject`、`record`。配置中的
外部 `command` 字段会被解析并保留接口，但不会默认执行不可信脚本。

## 飞书接入

### 1. 创建应用

1. 打开飞书开放平台，创建“企业自建应用”。
2. 在应用能力中启用“机器人”。
3. 记录应用的 `App ID` 和 `App Secret`，分别填入：

```env
FEISHU_APP_ID=cli_xxx
FEISHU_APP_SECRET=xxx
```

### 2. 配置事件订阅

GoClaw 使用飞书官方 SDK 的长连接模式：

1. 进入“事件订阅”。
2. 接收事件方式选择“使用长连接接收事件”。
3. 不需要配置公网回调 URL。
4. 订阅机器人接收消息所需事件。
5. 如果要使用审批卡片按钮，还需要确保应用能接收卡片交互事件。

当前代码会处理普通消息和审批卡片按钮：

- 普通消息进入 Agent。
- 审批卡片中的“允许”会转成 `/approve <审批ID>`。
- 审批卡片中的“拒绝”会转成 `/deny <审批ID>`。

### 3. 开通权限并发布应用

至少需要机器人接收和发送消息相关权限。配置后按飞书开放平台要求发布或安装应用到
目标组织，否则本地程序即使启动成功，也可能收不到消息或无法回复。

建议先只做单聊验证：

1. 把应用安装到当前企业。
2. 在飞书中找到机器人并发起私聊。
3. 发送 `/help`。

### 4. 配置允许的用户

GoClaw 默认只响应白名单用户。把允许使用机器人的飞书用户 ID 填入：

```env
FEISHU_ALLOWED_USER_IDS=ou_xxx
```

多个用户用英文逗号分隔：

```env
FEISHU_ALLOWED_USER_IDS=ou_xxx,ou_yyy
```

注意这里要填飞书事件里的用户 ID，通常形如 `ou_xxx`。如果填错，日志会出现
`Feishu message rejected`，机器人不会处理该用户消息。

### 5. 配置群聊，可选

群聊默认关闭。要启用群聊，需要同时配置：

```env
FEISHU_ENABLE_GROUPS=true
FEISHU_ALLOWED_GROUP_IDS=oc_xxx
```

多个群用英文逗号分隔：

```env
FEISHU_ALLOWED_GROUP_IDS=oc_xxx,oc_yyy
```

群聊中必须显式 `@bot`，GoClaw 不会响应 `@所有人`。

### 6. 完整 `.env` 示例

单聊模式：

```env
GOCLAW_CHANNEL=feishu
GOCLAW_WORKSPACE=/home/me/workspace/my-project
GOCLAW_DATA_DIR=.goclaw
GOCLAW_LOG_LEVEL=info
GOCLAW_MAX_STEPS=8
GOCLAW_BASH_TIMEOUT=10s
GOCLAW_BASH_OUTPUT_LIMIT=65536

LLM_API_KEY=sk-xxx
LLM_BASE_URL=https://api.example.com/v1
LLM_MODEL=your-model
LLM_TIMEOUT=120s

FEISHU_APP_ID=cli_xxx
FEISHU_APP_SECRET=xxx
FEISHU_ALLOWED_USER_IDS=ou_xxx

FEISHU_ENABLE_GROUPS=false
FEISHU_ALLOWED_GROUP_IDS=
```

群聊模式：

```env
GOCLAW_CHANNEL=feishu
GOCLAW_WORKSPACE=/home/me/workspace/my-project
GOCLAW_DATA_DIR=.goclaw

LLM_API_KEY=sk-xxx
LLM_BASE_URL=https://api.example.com/v1
LLM_MODEL=your-model

FEISHU_APP_ID=cli_xxx
FEISHU_APP_SECRET=xxx
FEISHU_ALLOWED_USER_IDS=ou_xxx
FEISHU_ENABLE_GROUPS=true
FEISHU_ALLOWED_GROUP_IDS=oc_xxx
```

### 7. 启动和验证

启动：

```bash
go run ./cmd/goclaw
```

验证顺序：

1. 看到 `Feishu channel ready`。
2. 私聊机器人发送 `/help`，应该收到命令说明。
3. 发送 `/status`，应该看到当前阶段、Todo 和最近错误摘要。
4. 发送 `读取 README.md 并总结`，应该收到模型回复。
5. 发送写文件类任务，应该收到工具审批卡片。
6. 点击“允许”或发送 `/approve <审批ID>`，Agent 应继续执行。

默认安全策略：

- 单聊仅允许 `FEISHU_ALLOWED_USER_IDS` 中的用户。
- 群聊默认关闭。
- 只有设置 `FEISHU_ENABLE_GROUPS=true` 且配置
  `FEISHU_ALLOWED_GROUP_IDS` 后才启用群聊。
- 群聊必须显式 `@bot`，不会响应 `@所有人`。

### 8. 常见问题

程序启动时报 `FEISHU_APP_ID is required when GOCLAW_CHANNEL=feishu`：

- `.env` 中缺少 `FEISHU_APP_ID`。
- 或当前 shell 中环境变量覆盖了 `.env`。

程序启动成功但机器人不回复：

- 确认飞书事件订阅选择了长连接模式。
- 确认应用已发布或安装到当前企业。
- 确认机器人能力已启用。
- 确认 `FEISHU_ALLOWED_USER_IDS` 是正确的 `ou_xxx`。
- 看日志是否有 `Feishu message rejected`。

群里不回复：

- 确认 `FEISHU_ENABLE_GROUPS=true`。
- 确认 `FEISHU_ALLOWED_GROUP_IDS` 填的是正确的 `oc_xxx`。
- 群聊必须显式 `@bot`。

审批卡片按钮没有反应：

- 确认应用已开启卡片交互事件。
- 可以用文本命令兜底：`/approve <审批ID>` 或 `/deny <审批ID>`。

模型不回复或报错：

- 先用 CLI 模式验证 `LLM_API_KEY`、`LLM_BASE_URL` 和 `LLM_MODEL`。
- 再切换 `GOCLAW_CHANNEL=feishu`。

飞书接入基于官方 SDK 的高层 Channel 模块：

- <https://github.com/larksuite/oapi-sdk-go/blob/v3_main/doc/channel.zh.md>

## 测试

```bash
make test
make test-race
make vet
```

普通测试完全离线，不需要真实模型或飞书凭据。

## 当前目录

```text
cmd/goclaw/                 程序入口
internal/agent/             Agent Loop
internal/tool/              工具注册表、bash 和文件工具
internal/permission/        权限规则和只读 Shell 分类
internal/hooks/             工具执行前后的 HookBus、配置和内置 Runner
internal/todo/              会话级 Todo 模型和持久化 Store
internal/subagent/          子 Agent 请求、结果、深度和并发限制
internal/skill/             Skill 模型、加载器和关键词选择器
internal/contextmgr/        会话历史、压缩策略、摘要器和持久化 Store
internal/memory/            长期记忆模型、Markdown Store、选择器和敏感信息检测
internal/prompt/            动态 System Prompt 构建器
internal/recovery/          错误分类和重试策略
internal/app/               命令路由和运行取消
internal/channel/           Channel 接口及 CLI/Fake/飞书实现
internal/config/            环境配置
internal/llm/               Eino 模型工厂
internal/server/            Channel 生命周期
internal/store/             JSON 状态、事件去重和审批 checkpoint
doc/plan.md                 长期实现计划
doc/chapters/               分阶段代码导读
```
