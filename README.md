# Codex Issue Worker

Python 标准库实现的单进程 GitHub Issue worker。只处理 `GH_REPO` 中带
`codex-task` 标签的开放 Issue；领取后切换为 `in-progress`，在独立克隆的
`worker/issue-<number>` 分支调用 Codex 或 Claude。执行模式会提交、推送任务分支并创建或复用 PR，
不会直接推送默认分支。失败时尝试添加 `worker-failed` 并留言。

默认是离线单次预览，不访问 GitHub、不启动 agent、不创建任务目录：

```bash
cd /home/agent/data/projects/codex-issue-worker
GH_REPO=owner/repository python3 -B -m src.worker.cli --dry-run --once
python3 -B -m unittest -v
systemd-analyze --user verify systemd/codex-issue-worker.service
```

Python 版本须支持现有源码的 f-string 语法（Python 3.12+）。真实执行另需 Git、已登录的
GitHub CLI 和所选 agent CLI；`AGENT_TMUX=1` 时需要 tmux。
`.env.example` 列出全部配置；CLI 不会自动加载此文件，systemd 从
`/home/agent/.config/codex-issue-worker/worker.env` 读取配置。

服务模板仅供 `agent` 的 systemd user manager 使用，默认 dry-run 成功后退出。
安装、登录、明确开启执行、日志、tmux 观察、停止与回滚见 [运维说明](docs/operations.md)。
当前交付仅包含配置与文档，不代表服务已经安装或启用。
