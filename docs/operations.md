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

`.env.example` 中各变量对应现有源码：

| 变量 | 含义 / 示例默认值 |
| --- | --- |
| `GH_REPO` | 必填，唯一授权的 `owner/repository`，使用前替换占位值 |
| `AGENT` | `codex` 或 `claude`，默认 `codex` |
| `TASK_LABEL` | 待处理标签，默认 `codex-task` |
| `IN_PROGRESS_LABEL` | 领取标签，默认 `in-progress`，不能与待处理标签相同 |
| `MAX_MINUTES` | 每任务总时间预算，正整数，默认 120 分钟 |
| `POLL_SECONDS` | 轮询间隔，正整数，默认 60 秒 |
| `WORK_ROOT` | agent 可写绝对路径，默认 `/home/agent/data/tasks` |
| `AGENT_TMUX` | `0` 直接捕获输出；`1` 使用实时 tmux 窗口 |

部署时，将非敏感配置存放在 `/home/agent/.config/codex-issue-worker/worker.env`，
权限设为 0600、所属用户为 agent。文件使用 `KEY=value`，不写 `export`，不依赖 shell
变量展开。不把 token、密码或私钥写入仓库或示例。不要打印现有凭据文件。
CLI 自身不加载 env 文件；手动运行时显式提供所需环境变量。

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
GH_REPO=owner/repository python3 -B -m src.worker.cli --dry-run --once
python3 -B -m unittest -v
systemd-analyze --user verify systemd/codex-issue-worker.service
```

无 fixture 时输出 `{"dry_run": true, "results": []}`。可以添加
`--issues-file /path/to/local-issues.json`；文件为 Issue 对象列表，包含
`number`、`title`、`url`、`state`、`labels`，其中 URL 必须匹配 GH_REPO，
例如 `https://github.com/owner/repository/issues/7`。这是本地数据，不会创建 Issue。
预览最多返回一个符合条件的分支，不调用 GitHub/agent，也不写任务日志。
离线通过不代表登录、网络或真实执行已经验证。

准备上述 env 文件后，以 agent 用户安装并验证默认 dry-run：

```bash
test "$(id -un)" = agent || exit 1
install -D -m 0644 systemd/codex-issue-worker.service /home/agent/.config/systemd/user/codex-issue-worker.service
systemctl --user daemon-reload
systemctl --user cat codex-issue-worker.service
systemctl --user start --wait codex-issue-worker.service
systemctl --user status codex-issue-worker.service --no-pager
systemctl --user show codex-issue-worker.service -p ConditionResult -p ActiveState -p SubState -p Result -p ExecMainCode -p ExecMainStatus -p ExecMainStartTimestamp -p NRestarts
journalctl --user -u codex-issue-worker.service -n 20 --no-pager
```

启动前检查 `cat` 输出没有覆盖 ExecStart 的执行模式 drop-in。
模板使用 `--dry-run --once`，成功后应为 `inactive (dead)`；`Restart=on-failure` 不会重启成功的预览。
此时 `systemctl status` 返回 3 是 inactive 状态的正常表现，不是 worker 退出码。
必须同时确认 `ConditionResult=yes`、`Result=success`、`ExecMainCode=1`（正常退出）、
`ExecMainStatus=0`、`NRestarts=0`，以及本次启动时间对应的 journal 包含
`{"dry_run": true, "results": []}`。仅有 inactive 或退出码 0 不能排除条件不匹配而跳过启动。
未启用的 inactive unit 可能被 manager 回收，导致后续 `show` 的时间戳为空、
`ConditionResult=no` 和 `ExecMainCode=0`；这不是本次执行的有效记录。
此时可在具备发行版 `python3-dbus` 的主机上保持 D-Bus 引用，再运行一次离线验证：

```bash
/usr/bin/python3 -B - <<'PY'
import dbus
import subprocess

bus = dbus.SessionBus()
manager = dbus.Interface(
    bus.get_object('org.freedesktop.systemd1', '/org/freedesktop/systemd1'),
    'org.freedesktop.systemd1.Manager')
unit = 'codex-issue-worker.service'
manager.RefUnit(unit)
try:
    subprocess.run(['systemctl', '--user', 'start', '--wait', unit], check=True)
    subprocess.run(['systemctl', '--user', 'status', unit, '--no-pager'])
    subprocess.run(['systemctl', '--user', 'show', unit,
                    '-p', 'ConditionResult', '-p', 'Result', '-p', 'ExecMainCode',
                    '-p', 'ExecMainStatus', '-p', 'NRestarts'], check=True)
finally:
    manager.UnrefUnit(unit)
PY
```

配置缺失等错误每 15 秒重试，300 秒内最多启动 5 次。

真实执行会修改 GitHub、提交和推送代码；只有单独授权后才在 user unit drop-in 中明确替换：

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/python3 -B -m src.worker.cli --execute
```

清空原 ExecStart 后再指定新命令。不要保留 `--once`，否则不是持续轮询。
没有 `EXECUTE` 环境开关，修改 worker.env 不会自动开启执行。
应用 drop-in 后需要 daemon-reload，再按下文停止/核查流程启动。
需要登录时自动启动或注销后继续运行属于后续部署决策，本次不执行 enable 或 linger。

## 状态与日志

```bash
systemctl --user status codex-issue-worker.service
journalctl --user -u codex-issue-worker.service -n 100 --no-pager
journalctl --user -u codex-issue-worker.service -f
```

worker 的 stdout（每轮 JSON 摘要和部分错误）及 stderr 进入 user journal。
journal 的持久化与保留由主机 journald 配置决定，不承诺固定文件路径。
执行任务的文件位于 `$WORK_ROOT/issue-<number>-<随机后缀>/`，默认根目录为
`/home/agent/data/tasks`：

| 文件 | 内容 |
| --- | --- |
| `events.jsonl` | JSONL 事件，含 task_id、时间、命令类别、退出码、最终状态；开始事件包含 session_name |
| `agent.stdout.log` | agent 原始 stdout，运行中持续写入 |
| `agent.stderr.log` | agent 原始 stderr，运行中持续写入 |
| `task.log` | 各命令完成后汇总 stdout/stderr；agent 运行中不一定更新 |
| `repo/` | 本任务独立克隆，保留供排查 |

每次重试生成新目录，不自动清理或轮转。日志和克隆会占用磁盘，资源限制不包含磁盘配额。
原始输出可能包含敏感任务数据，查看/分享前需检查，勿把完整输出贴到公开 Issue。
服务 UMask 为 0077；任务日志创建权限为 0600。

## AGENT_TMUX=1 时观察任务

修改配置为 `AGENT_TMUX=1` 后在安全停止并重新启动服务时生效。
默认 session_name 为 `codex-task-issue-<number>`，以 `events.jsonl` 中记录为准。
服务设置独立 `TMUX_TMPDIR=%t/codex-issue-worker`（%t 是 agent 的 user runtime 目录），
防止复用交互终端中已有的 tmux server，使新建 server 和子进程处于服务 cgroup 内。
以 agent 用户在具有 XDG_RUNTIME_DIR 的登录终端观察：

```bash
TMUX_TMPDIR="$XDG_RUNTIME_DIR/codex-issue-worker" tmux list-sessions
TMUX_TMPDIR="$XDG_RUNTIME_DIR/codex-issue-worker" tmux attach -r -t '=codex-task-issue-7'
```

将 7 替换为实际编号。`-r` 只读观察；按 `Ctrl-b` 然后 `d` 脱离，不停止任务。
不要在此专用 socket 下另建交互 session/server。任务结束后 session 通常消失，改查日志。
手动启动 CLI 未设置 TMUX_TMPDIR 时使用默认 tmux socket，attach 时需采用相同环境。

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
专用 tmux server 随服务停止，脱离 tmux 本身不等于停止服务。
不要通过删锁文件或杀所有 tmux 会话来停服务，也不要同时手动运行第二个 worker。
OS 锁在持锁进程退出时释放；它只保护共用 WORK_ROOT 的本机执行，不是跨主机分布式锁。

停止后检查任务日志、远端分支/PR 状态及 `in-progress` 标签是否遗留。
确认没有旧进程和已完成的结果后，才由操作员决定是否重新排队（恢复任务标签并移除领取标签）；
不要盲目重试，先核对是否已推送或创建 PR。强制中断前已完成的外部操作不会自动撤销。

回滚执行模式：先停止并核查，把 drop-in 的 ExecStart 改为模板中的
`/usr/bin/python3 -B -m src.worker.cli --dry-run --once`，daemon-reload 后重新预览。
回滚版本时保留现有目录与日志，使用经审核的历史版本独立部署，不 reset 或强推历史。
既有远端改动交由 PR 审查处理。不要自动删除克隆、日志或分支。

模板限制 CPU 为两核等效（200%）、内存 4 GiB、任务数 512、文件描述符 4096；
达到限制可能使任务失败/OOM，需结合 journal 与任务日志判断。user manager 的资源限制
是否实际生效还取决于主机 cgroup 支持；静态 verify 不能代替部署后的运行验证。
