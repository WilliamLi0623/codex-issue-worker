# Codex Issue Worker

Go 实现的持续运行 GitHub Issue worker。只处理 `GH_REPO` 中带 `codex-task` 标签的开放
Issue；领取后切换为 `in-progress`，在独立克隆的 `worker/issue-<number>` 分支调用 Codex
或 Claude。执行模式会提交、推送任务分支并创建或复用 PR，不会直接推送默认分支。失败时
尝试添加 `worker-failed` 并留言。`MAX_CONCURRENT_TASKS` 默认为 1，可设置有界并发。

默认是离线单次预览，不访问 GitHub、不启动 agent、不创建任务目录：

```bash
cd /home/agent/data/projects/codex-issue-worker
/home/agent/.local/opt/go1.27.1/bin/go test ./...
/home/agent/.local/opt/go1.27.1/bin/go test -race ./...
/home/agent/.local/opt/go1.27.1/bin/go vet ./...
systemd-analyze --user verify systemd/codex-issue-worker.service
```

Go 工具链用于构建和测试；服务运行时只需 Go worker 二进制、Git、已登录的 GitHub CLI
和所选 agent CLI。`.env.example` 列出配置；systemd 从
`/home/agent/.config/codex-issue-worker/worker.env` 读取配置。

Codex 默认使用 `AGENT_SANDBOX=workspace-write`，也支持 `read-only` 和
`danger-full-access`；此配置不改变 Claude 的权限模式。本隔离 VM 的 bubblewrap
在任务开始前报 `bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted`，
因此部署的 worker.env 显式使用 `AGENT_SANDBOX=danger-full-access`。该模式关闭
Codex 沙箱，依赖 VM 外部隔离；其他环境应保留默认值，纯读取任务可用 `read-only`。

服务模板仅供 `agent` 的 systemd user manager 使用，由 Go worker 持续轮询并执行任务。
启动服务前须确认授权仓库的 `codex-task` 队列为空。
服务继承 user manager 身份，通过 `ConditionUser=agent` 限定运行用户，不设置 `User=`。
安装、登录、日志观察与停止见 [运维说明](docs/operations.md)。
部署后须确认服务为 `active (running)`、进程用户为 `agent`，并检查本次启动日志无错误；
静态验证不能代替实际启动验证。
