# GoClaw

GoClaw 是一个使用 Go 和 Eino 构建的学习型编码 Agent。目标是从最小 Agent
Loop 开始，逐步实现工具、权限、Hooks、Todo、子 Agent、Skills、上下文压缩、
记忆、动态 System Prompt 和错误恢复，并通过飞书等 IM 操作本地工作区。

当前阶段：`s02-tool-use`

> s02 已建立工具注册表，并加入受工作区限制的文件读写、精确编辑和 glob 工具。
> 人工审批与完整权限管线将在 s03 实现。

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

## s02 已实现

- 环境变量配置和校验。
- OpenAI 兼容 Eino `AgenticModel` 工厂。
- 可直接阅读的 JSON 文件状态存储。
- Event ID 跨进程持久化去重。
- CLI、飞书 WebSocket 长连接和 Fake Channel。
- 流式回复抽象。
- `/help`、`/status`、`/new`、`/cancel`。
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
hello
```

也可以尝试：

```text
读取 README.md 并总结
创建 notes/example.txt，写入 hello，然后读回来
查找所有 **/*.go 文件
```

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
| `LLM_API_KEY` | 空 | OpenAI 兼容 API Key，启动时必填 |
| `LLM_BASE_URL` | 空 | OpenAI 兼容 API Base URL |
| `LLM_MODEL` | 空 | 模型名称，启动时必填 |
| `LLM_TIMEOUT` | `120s` | 单次模型请求超时 |

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
internal/app/               命令路由和运行取消
internal/channel/           Channel 接口及 CLI/Fake/飞书实现
internal/config/            环境配置
internal/llm/               Eino 模型工厂
internal/server/            Channel 生命周期
internal/store/             JSON 状态和事件去重
doc/plan.md                 长期实现计划
doc/chapters/               分阶段代码导读
```
