# 任务观察层

`TaskRunner.run(issue, task_id=None)` 默认使用 `issue-<number>`，重试不改变 ID。
自定义 ID 保留在 `TaskResult.task_id` 和事件中；session 名将非 ASCII
字母、数字、下划线、连字符替换为 `-`，去除两端 `-`，最多保留 80 字符，
空结果使用 `task`，然后加 `codex-task-` 前缀。不同 ID 清理后可能重名；
已有同名 session 时启动失败，不接管或终止它。

每次实际执行在 `WORK_ROOT/issue-<number>-<随机后缀>/` 保留独立目录：

- `agent.stdout.log`、`agent.stderr.log`：agent 原始双流，执行期间持续写入。
- `events.jsonl`：任务开始/结束、命令开始/结束；包含任务 ID、时间、命令类别、
  退出码和超时状态，不记录命令参数、环境变量或输出正文。
- `task.log`：兼容既有调用者的混合文本日志。
- `repo/`：原有任务 checkout；tmux 完成标记位于私有 `tmux-*/` 子目录。

任务目录权限为 0700，日志创建权限为 0600。日志保留 agent 原始输出，
不做内容脱敏，也不读取或复制凭据。不要让 agent 将凭据输出到终端。

配置 `AGENT_TMUX=1` 启用 tmux（默认 `0`，其他值拒绝）。运行中的默认任务可通过
`tmux attach -t codex-task-issue-7` 观察；`Ctrl-b d` 脱离观察。
agent 仍是非交互执行，双流同时显示在 pane 中，文件保持分离。
正常结束后 session 自动退出，日志留存。无需真实 GitHub 即可运行本仓库测试。

`TaskResult` 暴露 `task_id`、`session_name`、`stdout_path`、`stderr_path`、
`events_path`，原字段保持兼容。CLI 的既有 JSON 摘要格式不变。
离线 `run_cycle(dry_run=True, issues=...)` 返回默认任务身份，不创建日志或启动 tmux。
注入旧 `process.run` 时仍支持旧测试替身；单独注入的 `agent_executor` 接收双流路径
及 `session_name`、`tmux`，负责写入原始日志并返回 `CommandResult`。

tmux 子进程负责 agent 超时和进程组收尾，runner 等待完成标记；标记缺失时
最多额外等待 5 秒后报告超时并保留部分日志。未实现重连恢复、结束后保留 session、
日志轮转、取消接口或 agent 脱离进程组后的进程追踪。tmux 需要本机安装；
测试会在缺少 tmux 时跳过真实 tmux 检查。此层不改变原有 execute 发布流程。

离线预览（本地 fixture 为 GitHub Issue JSON 列表）：

```sh
GH_REPO=x/y WORK_ROOT=.test-artifacts/preview AGENT_TMUX=1 \
  python3 -B -m src.worker.cli --dry-run --once --issues-file .test-artifacts/offline-issues.json
```
