# GoClaw

GoClaw 是一个使用 Go 和 Eino 构建的学习型编码 Agent。目标是从最小 Agent
Loop 开始，逐步实现工具、权限、Hooks、Todo、子 Agent、Skills、上下文压缩、
记忆、动态 System Prompt 和错误恢复，并通过飞书等 IM 操作本地工作区。

当前阶段：`s00-bootstrap`

> s00 只建立工程外壳和 IM 通道，尚未执行模型推理。最小 Agent Loop 将在 s01
> 实现。

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

## s00 已实现

- 环境变量配置和校验。
- OpenAI 兼容 Eino `AgenticModel` 工厂。
- 可直接阅读的 JSON 文件状态存储。
- Event ID 跨进程持久化去重。
- CLI、飞书 WebSocket 长连接和 Fake Channel。
- 流式回复抽象。
- `/help`、`/status`、`/new`、`/cancel`。
- 系统信号驱动的安全退出。

## 快速开始

环境要求：

- Go 1.24 或更高版本。

运行本地 CLI：

```bash
go run ./cmd/goclaw
```

输入：

```text
/help
/status
/new
hello
```

运行状态默认保存在当前工作区的 `.goclaw/`，该目录不会提交到 Git。

## 配置

复制环境变量模板并按需填写：

```bash
cp .env.example .env
set -a
source .env
set +a
```

主要变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `GOCLAW_CHANNEL` | `cli` | `cli` 或 `feishu` |
| `GOCLAW_WORKSPACE` | `.` | Agent 允许操作的工作区 |
| `GOCLAW_DATA_DIR` | `.goclaw` | 运行状态目录 |
| `GOCLAW_LOG_LEVEL` | `info` | `debug/info/warn/error` |
| `LLM_API_KEY` | 空 | OpenAI 兼容 API Key，s01 开始使用 |
| `LLM_BASE_URL` | 空 | OpenAI 兼容 API Base URL |
| `LLM_MODEL` | 空 | 模型名称 |
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
internal/app/               基础命令和运行取消
internal/channel/           Channel 接口及 CLI/Fake/飞书实现
internal/config/            环境配置
internal/llm/               Eino 模型工厂
internal/server/            Channel 生命周期
internal/store/             JSON 状态和事件去重
doc/plan.md                 长期实现计划
doc/chapters/s00.md         本章代码导读
```
