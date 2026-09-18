# 运维说明

## 配置与登录

所有运行和管理命令均在 `agent` 用户会话内执行，使用 `systemctl --user`，不要以 root
安装为系统服务。服务继承 user manager 的用户和组身份，通过 `ConditionUser=agent`
检查 manager 是否以 agent 运行；不匹配时跳过启动。不要设置 `User=`、`Group=` 或
`SupplementaryGroups=`：显式 `User=` 会触发附加组初始化，在无权限的 user manager
中可能于 ExecStart 前报 `216/GROUP` 和 `Failed to determine supplementary groups: Operation not permitted`。
参见 [systemd.exec 用户身份语义](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#User=)
及 [ConditionUser 条件](https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html#ConditionUser=)。
固定工作目录是 `/home/agent/data/projects/codex-issue-worker`；迁移仓库必须同步修改 unit。

`.env.example` 中各变量对应 Go worker。任务必须带 `codex-task` 标签，且作者关联必须为 `OWNER`、`MEMBER` 或 `COLLABORATOR`；其他作者的 Issue 会在 agent 启动及领取标签变更前跳过：

| 变量 | 含义 / 示例默认值 |
| --- | --- |
| `GH_REPO` | 必填，唯一授权的 `owner/repository`，使用前替换占位值 |
| `AGENT` | `codex` 或 `claude`，默认 `codex` |
| `AGENT_SANDBOX` | 仅用于 Codex：`read-only`、`workspace-write`（默认）或 `danger-full-access`；非法值在启动时拒绝 |
| `TASK_LABEL` | 待处理标签，默认 `codex-task` |
| `IN_PROGRESS_LABEL` | 领取标签，默认 `in-progress`，不能与待处理标签相同 |
| `MAX_MINUTES` | 每任务总时间预算，正整数，默认 120 分钟 |
| `POLL_SECONDS` | 轮询间隔，正整数，默认 60 秒 |
| `MAX_CONCURRENT_TASKS` | 同时运行任务上限，正整数，默认 1；满载时留待后续轮询 |
| `COMPLETED_TASK_RETENTION` | 保留的最新已完成任务目录数，正整数，默认 10；运行中或未完成目录不会清理 |
| `WORK_ROOT` | agent 可写绝对路径，默认 `/home/agent/data/tasks` |
| `FAILED_LABEL` | 失败标签，默认 `worker-failed` |
| `AUTO_MERGE` | 是否为 worker 创建或恢复的 PR 请求 GitHub 保护性自动合并，默认 `false`；请求使用 squash，不绕过评审、检查或冲突保护 |

部署时，将非敏感配置存放在 `/home/agent/.config/codex-issue-worker/worker.env`，
权限设为 0600、所属用户为 agent。文件使用 `KEY=value`，不写 `export`，不依赖 shell
变量展开。不把 token、密码或私钥写入仓库或示例。不要打印现有凭据文件。
CLI 自身不加载 env 文件；手动运行时显式提供所需环境变量。

启用 `AUTO_MERGE=true` 前，确认仓库的分支保护和自动合并策略符合预期。worker 使用
`gh pr merge <url> --auto --squash` 请求 GitHub 正常的保护性流程，不直接合并、不自动批准
评审、不删除分支，也不强制推送。请求被 GitHub 拒绝时，事件日志会记录拒绝原因，但任务仍
会成功返回 PR URL；查看 `events.jsonl` 中的 `auto_merge_skipped`、`auto_merge_requested` 或
`auto_merge_rejected` 事件。
Worker smoke test exercises `AUTO_MERGE`.

### Codex 沙箱与本 VM

worker 将 `AGENT_SANDBOX` 显式传给 `codex exec --sandbox`，不自动降级。
一般环境保持 `.env.example` 的 `workspace-write`；仅需读取的任务可选 `read-only`。
Claude 仍使用原有 `acceptEdits` 权限模式。

本隔离 VM 处理 Issue #1 时，bubblewrap 在任何任务工作前失败：
`bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted`。
部署文件 `/home/agent/.config/codex-issue-worker/worker.env` 因此显式设置：

```ini
AGENT_SANDBOX=danger-full-access
```

此模式关闭 Codex 沙箱，进程可访问 agent 用户可访问的文件及网络，依赖外部 VM 隔离；
仓库限定、任务分支检查和默认分支保护不变，但不能代替沙箱。不要在普通共享主机上
照搬此设置，也不要通过放宽 systemd 权限来绕过此故障。
修改 env 文件不会改变已运行进程；按下文安全停止流程，在任务结束、检查队列后
重新启动服务才生效。无需仅因 env 值变更执行 daemon-reload。
恢复沙箱时将值改回 `workspace-write` 并按同样流程重启。
本修复的单元测试和 systemd 静态验证不代表已重跑 Issue #1；重新排队需另行授权，
避免重复提交、推送或创建 PR。

### Codex 模型默认值

worker 调用 `codex exec` 时只设置工作目录和沙箱，不传 `--model`、`--profile` 或
`--reasoning-effort` 覆盖。因此，Codex 使用 agent 用户的 `~/.codex/config.toml`
中的默认设置。修改此配置只影响后续 Codex 调用，不会改变已经运行的 Codex 会话。
当前未实现按任务自动路由模型。

在 agent 的交互终端用 `gh auth login` 完成 GitHub 登录，并使用所选 agent CLI 的交互登录流程。
登录应由操作员完成，不把凭据放进命令行、日志或报告。确认 Git 的提交身份和 HTTPS
认证可供非交互进程使用。准备 `codex-task`、`in-progress`、`worker-failed` 标签；
标签变更和真实执行应在单独授权后进行。本次包装不创建标签、Issue 或 PR。

user service 不加载交互 shell 配置。模板 PATH 包含 `/home/agent/.local/bin`、
`/usr/local/bin`、`/usr/bin`、`/bin`；如果工具位于其他目录，部署时明确调整 PATH。
`NoNewPrivileges=yes` 禁止获得新权限；依赖提权的任务会失败。

## 离线验证与 user service 部署

在仓库中运行：

```bash
/home/agent/.local/opt/go1.27.1/bin/go test ./...
/home/agent/.local/opt/go1.27.1/bin/go test -race ./...
/home/agent/.local/opt/go1.27.1/bin/go vet ./...
systemd-analyze --user verify systemd/codex-issue-worker.service
```

测试不访问 GitHub，也不创建 Issue。离线通过不代表登录、网络或真实执行已经验证。

准备上述 env 文件后，以 agent 用户安装持续执行服务。Go worker 启动后持续轮询；
Go worker 使用 `WORK_ROOT/worker.lock` 防止同一工作根目录下的重复实例并发认领任务。

```bash
test "$(id -un)" = agent || exit 1
install -D -m 0644 systemd/codex-issue-worker.service /home/agent/.config/systemd/user/codex-issue-worker.service
systemctl --user daemon-reload
systemctl --user cat codex-issue-worker.service
```

启动前检查有效 ExecStart 指向 `/home/agent/.local/bin/codex-issue-worker-go`，
确认 `ConditionUser=agent` 且没有 `User=`。将下面 GH_REPO 替换为 worker.env 中的授权仓库；
TASK_LABEL 若有定制，也须检查对应队列。只读取 Issue，查询失败或队列非空时不要继续启动：

```bash
GH_REPO=owner/repository
queue=$(gh issue list --repo "$GH_REPO" --state open --label codex-task --limit 1 --json number) || exit 1
test "$queue" = '[]' || exit 1
started_at=$(date --iso-8601=seconds)
systemctl --user start codex-issue-worker.service
systemctl --user status codex-issue-worker.service --no-pager
systemctl --user show codex-issue-worker.service -p ConditionResult -p ActiveState -p SubState -p Result -p MainPID -p ExecMainStatus -p NRestarts
ps -o user=,pid=,args= -p "$(systemctl --user show codex-issue-worker.service -p MainPID --value)"
journalctl --user -u codex-issue-worker.service --since "$started_at" --no-pager
```

确认 `ConditionResult=yes`、`ActiveState=active`、`SubState=running`、`Result=success`、
`ExecMainStatus=0`、`NRestarts=0`，MainPID 的用户为 agent、命令指向 Go 二进制。
至少等待一个 `POLL_SECONDS` 间隔后，再确认服务仍运行且日志无错误。仅 active 不能证明
首次 GitHub 查询成功。
队列检查只是启动时的快照，之后新加入的任务会被自动执行。
配置缺失等错误每 15 秒重试，300 秒内最多启动 5 次。
需要登录时自动启动或注销后继续运行属于后续部署决策，本次不执行 enable 或 linger。

## 状态与日志

### 远程状态摘要

在操作员机器构建并运行只读状态客户端；将目标替换为可通过 SSH 登录的 worker
主机，不要把密码、token 或私钥作为参数传入：

```bash
go build -o codex-issue-worker-status ./cmd/status
./codex-issue-worker-status --host agent@worker.example.com
```

客户端在远端固定检查 `codex-issue-worker.service`、部署 checkout
`/home/agent/data/projects/codex-issue-worker`、`worker.env` 中的 `WORK_ROOT`、
`GH_REPO` 和 `TASK_LABEL`，并查询当前开放且带任务标签的 Issue。输出字段为：

- `Service`：`ActiveState`、`SubState`、`Result` 和 `MainPID`。
- `Checkout`：部署 checkout 的路径。
- `Commit`：该 checkout 当前 `HEAD`。
- `Active task directories`：`WORK_ROOT` 下匹配 `issue-*` 的一级目录名；不包含日志内容。
- `Queue (<TASK_LABEL>)`：Issue 编号、标题和 URL。

这个命令只执行 `systemctl --user show`、`git rev-parse`、`find` 和
`gh issue list`；不会停止或重启服务、修改标签、删除目录、重新排队任务或读取任务日志。

检查 worker 服务状态时运行：

```bash
systemctl --user status codex-issue-worker.service --no-pager
journalctl --user -u codex-issue-worker.service -n 100 --no-pager
journalctl --user -u codex-issue-worker.service -f
```

worker 的任务状态摘要和错误进入 user journal。
journal 的持久化与保留由主机 journald 配置决定，不承诺固定文件路径。
执行任务的文件位于 `$WORK_ROOT/issue-<number>-<随机后缀>/`，默认根目录为
`/home/agent/data/tasks`：

| 文件 | 内容 |
| --- | --- |
| `events.jsonl` | JSONL 事件，含时间、Issue、分支、命令类别、退出码和最终状态 |
| `agent.stdout.log` | agent 原始 stdout，agent 命令结束后写入 |
| `agent.stderr.log` | agent 原始 stderr，agent 命令结束后写入 |
| `task.log` | 各命令完成后汇总 stdout/stderr；agent 运行中不一定更新 |
| `repo/` | 本任务独立克隆，保留供排查 |

每次重试生成新目录。任务完成且所有日志文件关闭后，worker 只保留最近
`COMPLETED_TASK_RETENTION` 个已完成目录，并删除更旧的已完成目录及其中的日志和克隆。
任务运行期间目录带有 `.task-active` 标记；只有写入 `.task-completed` 标记的目录才会参与清理，
因此运行中和 worker 中断后未完成的目录不会被自动删除。中断目录可能持续占用磁盘，操作员应先
检查任务状态、远端分支和 PR，再手动归档或删除；不要删除正在运行任务的目录。
日志和克隆仍会占用磁盘，资源限制不包含磁盘配额。原始输出可能包含敏感任务数据，查看/分享前需检查，
勿把完整输出贴到公开 Issue。
服务 UMask 为 0077；任务日志创建权限为 0600。

## 观察任务输出

Go worker 将 agent 输出保存在任务目录。以 agent 用户查看当前 Issue 的文件：

```bash
find /home/agent/data/tasks -maxdepth 2 -path '*/issue-7-*/agent.stdout.log' -print
tail -F /home/agent/data/tasks/issue-7-<suffix>/agent.stdout.log
```

将编号和 `<suffix>` 替换为实际任务目录。stderr 与事件流位于同一目录的
`agent.stderr.log` 和 `events.jsonl`，可用 `tail -F` 实时查看。

## 安全停止、恢复与回滚

优先等当前任务结束再停止，避免留下已领取但未完成的 Issue。
需要立即阻止继续处理时运行：

```bash
systemctl --user stop codex-issue-worker.service
systemctl --user status codex-issue-worker.service
systemctl --user show codex-issue-worker.service -p ActiveState -p SubState -p MainPID -p ControlGroup
```

`KillMode=control-group` 向服务及其子进程发送终止信号，30 秒后仍未退出的进程由 systemd
强制结束；显式 stop 不触发 on-failure 重启。这不是业务级优雅取消：现有 worker 没有
SIGTERM 清理处理，可能没有 task_finished 事件、失败留言或标签恢复。
不要通过删除锁文件来停服务，也不要同时手动运行第二个 worker。
OS 锁在持锁进程退出时释放；它只保护共用 WORK_ROOT 的本机执行，不是跨主机分布式锁。

停止后检查任务日志、远端分支/PR 状态及 `in-progress` 标签是否遗留。
确认没有旧进程和已完成的结果后，才由操作员决定是否重新排队（恢复任务标签并移除领取标签）；
不要盲目重试，先核对是否已推送或创建 PR。强制中断前已完成的外部操作不会自动撤销。

该 Go-only 版本不包含旧 worker 回滚路径。

模板限制 CPU 为两核等效（200%）、内存 4 GiB、任务数 512、文件描述符 4096；
达到限制可能使任务失败/OOM，需结合 journal 与任务日志判断。user manager 的资源限制
是否实际生效还取决于主机 cgroup 支持；静态 verify 不能代替部署后的运行验证。
