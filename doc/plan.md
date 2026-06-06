# GoClaw 实现计划

## 1. 项目目标

GoClaw 是一个使用 Go 和 Eino 构建的学习型编码 Agent。它通过飞书等 IM
接收任务，在受限工作区内读取、修改文件并执行受控命令。

项目以大二日常实习所需能力为学习目标，重点不是快速堆叠功能，而是理解并实现
Agent Harness 的关键机制。每个阶段必须包含实现、测试、文档、Git 提交和标签。

参考项目：

- <https://github.com/shareAI-lab/learn-claude-code>
- 采用其根目录新版教程的前 11 章，不使用旧版网页章节编号。

## 2. 已确定的技术决策

- 语言：Go，最低版本 1.24。
- Agent 框架：Eino。模型、消息 Schema、工具 Schema 和流式基础设施使用 Eino；
  Agent Loop、权限、Hooks、上下文和记忆等教学重点自行实现。
- 模型：OpenAI 兼容 Chat Completions 接口，通过 API Key、Base URL 和 Model 配置。
- 首个真实 IM：飞书官方 Go SDK Channel 模块，通过 WebSocket 长连接接收消息。
- 产品定位：限定本地工作区的编码助手。
- 状态存储：JSON、JSONL 和 Markdown 文件，优先保证可读性。
- 教学历史：维护一条持续演进的主线，每章使用细粒度提交和 Git 标签冻结。
- 交付节奏：每章实现完成后暂停，等待阅读和验收。

计划依赖版本：

- `github.com/cloudwego/eino v0.9.1`
- `github.com/cloudwego/eino-ext/components/model/agenticopenai v0.2.1`
- `github.com/larksuite/oapi-sdk-go/v3 v3.9.4`

`agenticopenai v0.2.1` 与 Eino `v0.9.1` 对齐，并通过请求级
`model.WithTools` 传入工具，避免旧 `BindTools` 接口的并发写入问题。

## 3. 总体架构

稳定的核心边界：

- `Channel`：接收统一消息、发送回复、创建流式回复。
- `AgentRunner`：启动或恢复一次 Agent 运行。
- `RunResult`：表示 `completed`、`waiting_approval`、`cancelled` 或 `failed`。
- `Tool` 与 `ToolRegistry`：描述工具 Schema、执行函数、安全性和并发元数据。
- `PolicyEngine`：返回 `allow`、`ask`、`deny` 或 `invalid`。
- `HookBus`：承载 Agent 和工具生命周期扩展点。
- `SessionStore`：持久化历史、Todo、审批、转录和记忆。

默认运行数据位于 `.goclaw/`：

- `events/`：IM Event ID 去重记录。
- `sessions/`：会话元数据和状态。
- `transcripts/`：完整会话转录。
- `tool-results/`：超大工具结果。
- `memory/`：Markdown 长期记忆。

安全默认值：

- 文件权限 `0600`，目录权限 `0700`。
- 状态文件使用同目录临时文件加原子重命名写入。
- 相同 IM 会话串行执行，不同会话可以并发。
- 飞书单聊只允许白名单用户。
- 飞书群聊默认关闭；启用后要求群白名单并显式 `@bot`。
- 工作区之外的路径和符号链接逃逸必须拒绝。

## 4. 阶段路线

### s00 工程启动

- 建立配置加载、结构化日志、可取消进程和基础错误处理。
- 建立 CLI、飞书和 Fake Channel。
- 实现文件状态存储、持久化 Event ID 去重和会话元数据。
- 实现 `/help`、`/status`、`/new`、`/cancel`。
- 建立 OpenAI 兼容 Eino 模型工厂，但不实现 Agent Loop。
- 标签：`s00-bootstrap`。

### s01 Agent Loop

- 手写“模型调用 -> 工具调用 -> 工具结果回填 -> 再调用”循环。
- 只提供一个带超时、输出上限和硬拒绝规则的 `bash` 工具。
- 测试纯文本结束、多轮工具调用、未知内容块、步数上限和取消。
- 标签：`s01-agent-loop`。

### s02 Tool Use

- 增加 `read_file`、`write_file`、`edit_file` 和 `glob`。
- 建立工具注册表与统一参数校验。
- 只读工具允许并发，写工具顺序执行，并保持结果顺序。
- 测试路径限制、符号链接逃逸、精确编辑和并发行为。
- 标签：`s02-tool-use`。

### s03 Permission

- 建立硬拒绝、规则判断、参数校验和人工审批四层权限管线。
- 读操作自动允许；写文件、编辑文件和非明确只读 Shell 命令需要审批。
- 飞书审批卡片为主，文本审批命令作为后备。
- 审批持久化，并支持进程重启后恢复。
- 标签：`s03-permission`。

### s04 Hooks

- 加入 `PreToolUse` 和 `PostToolUse` 工具生命周期事件。
- Hook 支持精确工具名和 `*` 通配匹配。
- `PreToolUse` 支持放行、阻断和注入提示。
- `PostToolUse` 支持观察工具结果并注入提示。
- Hook 不得绕过硬拒绝规则、非法参数校验或人工审批。
- 默认只启用安全内置 Runner，外部 command Hook 保留接口但不默认执行。
- 标签：`s04-hooks`。

### s05 TodoWrite

- 增加按会话持久化的 `todo_write`。
- Todo 状态为 `pending`、`in_progress`、`completed`。
- 最多一个 Todo 处于执行中，连续三轮未更新时注入提醒。
- `/status` 展示目标、Todo、待审批和上下文状态。
- 标签：`s05-todo-write`。

### s06 Subagent

- 增加同步 `task` 工具，为子 Agent 创建独立消息历史。
- 子 Agent 默认只读、继承权限规则、禁止递归委派。
- 父 Agent 只接收子 Agent 的最终摘要。
- 测试上下文隔离、权限继承、超时和取消。
- 标签：`s06-subagent`。

### s07 Skill Loading

- 从 `skills/<name>/SKILL.md` 读取 YAML frontmatter。
- System Prompt 只注入名称和描述。
- 通过 `load_skill` 按需加载完整正文。
- 拒绝重复名称、非法路径和损坏 frontmatter。
- 标签：`s07-skill-loading`。

### s08 Context Compact

- 大于 32 KiB 的工具结果落盘并保留预览。
- 单轮工具结果上下文预算为 128 KiB。
- 分层执行旧结果压缩、中间历史裁剪和 LLM 摘要。
- 达到配置上下文窗口 75% 时自动压缩。
- 压缩前保存完整转录；上下文超限只应急重试一次。
- 标签：`s08-context-compact`。

### s09 Memory

- 实现 `user`、`feedback`、`project`、`reference` 四类 Markdown 记忆。
- 每轮按索引选择相关记忆，成功结束后提取新记忆。
- 等待审批或执行失败时不提取记忆。
- 达到阈值后合并去重，旧文件归档而不是直接删除。
- 标签：`s09-memory`。

### s10 System Prompt

- Prompt 拆为 identity、safety、workspace、tools、channel、todo、
  skills、memory 和 summary 段。
- 根据真实运行状态启用段落，保持固定顺序。
- 使用内容哈希缓存，禁止根据用户关键词猜测模式。
- 使用快照测试验证顺序、条件段和缓存失效。
- 标签：`s10-system-prompt`。

### s11 Error Recovery

- 分类处理限流、网络超时、5xx、上下文超限、输出截断和永久鉴权错误。
- 支持 `Retry-After`、指数退避加抖动、一次应急压缩和最多两次续写。
- 连续过载三次后可以切换 `LLM_FALLBACK_MODEL`。
- 不自动重试结果未知的写工具。
- 飞书显示简洁恢复状态，最终错误保留追踪 ID。
- 标签：`s11-error-recovery`。

## 5. 测试和 Git 规则

每章必须通过：

```bash
go test ./...
go test -race ./...
go vet ./...
```

测试默认完全离线，使用 Fake Model、Fake Channel、临时工作区、假时钟和确定性
随机源。真实模型和飞书集成测试只能通过显式环境变量开启。

每章拆成 2 至 4 个始终可构建、可测试的提交，提交前缀使用：

- `feat:`
- `test:`
- `docs:`
- `refactor:`
- `fix:`

章节完成后更新本文件的进度、创建带说明的 Git 标签，然后暂停等待验收。

## 6. 暂不包含

前 11 章不实现：

- Heartbeat
- Cron
- 后台任务
- Agent Teams
- Worktree 隔离
- MCP
- 向量数据库
- Web UI
- 多租户

## 7. 当前进度

- [x] s00 工程启动
- [x] s01 Agent Loop
- [x] s02 Tool Use
- [x] s03 Permission
- [x] s04 Hooks
- [ ] s05 TodoWrite
- [ ] s06 Subagent
- [ ] s07 Skill Loading
- [ ] s08 Context Compact
- [ ] s09 Memory
- [ ] s10 System Prompt
- [ ] s11 Error Recovery
