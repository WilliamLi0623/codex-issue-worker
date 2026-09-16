# GitHub Issue 驱动的 Codex / Claude Worker

这是一个用 Go 编写的 Linux 常驻服务：轮询指定 GitHub 仓库中带任务标签、且尚未标记为处理中的开放 Issue，为每项任务创建独立工作目录和 `worker/issue-<number>` 分支，再调用 Codex CLI 或 Claude CLI。任务成功后，worker 推送任务分支并创建或复用 Pull Request；它不会直接推送默认分支。

## Requirements

- Linux；仓库包含 Linux 专用的进程管理和文件锁实现。
- Go 1.23 或更新版本。
- Git 与 GitHub CLI（`gh`），并已登录有目标仓库访问权限的 GitHub 账号。
- Codex CLI 或 Claude CLI，并已完成相应的交互式登录。

## Installation

```bash
git clone https://github.com/WilliamLi0623/codex-issue-worker.git
cd codex-issue-worker
go build -o codex-issue-worker ./cmd/worker
```

## Configuration

worker 从进程环境读取设置，不会自行加载 `.env` 文件。[`.env.example`](.env.example) 仅提供非敏感配置示例；部署服务时应由服务管理器加载配置。

| 变量 | 用途 | 默认值 |
| --- | --- | --- |
| `GH_REPO` | 唯一授权的 GitHub `owner/repository`，必填 | 无 |
| `AGENT` | 执行任务的 CLI：`codex` 或 `claude` | `codex` |
| `AGENT_SANDBOX` | Codex 沙箱模式：`read-only`、`workspace-write` 或 `danger-full-access` | `workspace-write` |
| `TASK_LABEL` | worker 领取任务的 Issue 标签 | `codex-task` |
| `IN_PROGRESS_LABEL` | 执行期间使用的标签 | `in-progress` |
| `FAILED_LABEL` | 失败时添加的标签 | `worker-failed` |
| `MAX_MINUTES` | 单个任务的总时限（正整数，分钟） | `120` |
| `POLL_SECONDS` | 队列轮询间隔（正整数，秒） | `60` |
| `MAX_CONCURRENT_TASKS` | 并发任务数上限（正整数） | `1` |
| `WORK_ROOT` | 保存任务克隆、锁和日志的目录 | `/home/agent/data/tasks` |

`danger-full-access` 会关闭 Codex 沙箱；仅应在具备外部隔离的环境中使用。该设置不改变 Claude 的权限模式。不要把凭据写入仓库或任务 Issue。

## Quick start

先确认目标仓库和队列确实适合自动执行：worker 启动后会持续轮询所有匹配标签的开放 Issue，并可能推送分支、创建 Pull Request。完成 GitHub CLI 与所选 agent CLI 登录后，在 Linux shell 中设置目标仓库并启动：

```bash
export GH_REPO=owner/repository
./codex-issue-worker
```

该命令以前述构建步骤生成的二进制启动常驻 worker；默认每 60 秒轮询一次。首次启动前应阅读[运维说明](operations.md)，并确认任务标签、Git 提交身份、认证和服务配置。

## Testing

在 Linux 仓库根目录运行：

```bash
go test ./...
go test -race ./...
go vet ./...
```

## Documentation

- [运维说明](operations.md)：配置、systemd user service、日志、任务观察、安全停止与回滚。
- [systemd user service 模板](codex-issue-worker.service)

