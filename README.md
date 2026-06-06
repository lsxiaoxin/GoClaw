# GoClaw

GoClaw 是一个使用 Go 和 Eino 构建的学习型编码 Agent。目标是从最小 Agent
Loop 开始，逐步实现工具、权限、Hooks、Todo、子 Agent、Skills、上下文压缩、
记忆、动态 System Prompt 和错误恢复，并通过飞书等 IM 操作本地工作区。

当前阶段：`s07-skill-loading`

> s07 已加入本地 Skills 加载。GoClaw 会从 `.goclaw/skills/` 读取技能说明，
> 根据用户消息选择相关 skill，并通过 `load_skill` 按需加载完整正文。

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

## 快速开始

环境要求：

- Go 1.24 或更高版本。

运行本地 CLI：

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

写文件、编辑文件或执行非明确只读的 bash 时，GoClaw 会先要求人工审批。
`todo_write` 作为普通工具执行，也会经过权限和 Hook 管线。
`task` 默认用于只读调查，父 Agent 只会看到子 Agent 的摘要。
`load_skill` 用于读取已配置 skill 的完整说明。

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

1. 在飞书开放平台创建企业自建应用并启用机器人能力。
2. 将事件订阅方式设置为“使用长连接接收事件”。
3. 订阅接收消息所需事件，并为应用开通相应消息权限。
4. 设置 `FEISHU_APP_ID`、`FEISHU_APP_SECRET` 和
   `FEISHU_ALLOWED_USER_IDS`。
5. 设置 `GOCLAW_CHANNEL=feishu` 后启动程序。

默认安全策略：

- 单聊仅允许 `FEISHU_ALLOWED_USER_IDS` 中的用户。
- 群聊默认关闭。
- 只有设置 `FEISHU_ENABLE_GROUPS=true` 且配置
  `FEISHU_ALLOWED_GROUP_IDS` 后才启用群聊。
- 群聊必须显式 `@bot`，不会响应 `@所有人`。

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
internal/app/               命令路由和运行取消
internal/channel/           Channel 接口及 CLI/Fake/飞书实现
internal/config/            环境配置
internal/llm/               Eino 模型工厂
internal/server/            Channel 生命周期
internal/store/             JSON 状态、事件去重和审批 checkpoint
doc/plan.md                 长期实现计划
doc/chapters/               分阶段代码导读
```
