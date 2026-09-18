# Codex Multi-Agent over WebCodex

Codex is the orchestration/control plane; WebCodex is the remote execution plane. Codex owns decomposition, spawning, concurrency, dependencies, waiting, reasoning-layer retries, integration decisions and aggregation. WebCodex owns projects, isolated workspaces, edits, Git, processes, validation Jobs and remote execution state.

## Normal execution
```mermaid
flowchart TD
U[User]-->O[Codex orchestrator]
O-->E[wc_explorer]
O-->A[wc_worker A]
O-->B[wc_worker B]
E-->M[WebCodex MCP]
A-->M
B-->M
M-->S[WebCodex Server]
S-->R[Proxmox Runner]
A-->I[wc_integrator]
B-->I
I-->V[wc_validator]
V-->H[Human review]
```

## Write-worker isolation
```mermaid
flowchart LR
SRC[Read-only immutable base]-->WA[Worker A worktree]
SRC-->WB[Worker B worktree]
WA-->CA[Commit A]
WB-->CB[Commit B]
```

## Integration
```mermaid
flowchart LR
B[Original base]-->IW[Integration worktree]
A[Worker commit A]-->IW
C[Worker commit B]-->IW
IW-->IC[Integration commit]
IC-->V[Independent validator]
```

## Crash recovery
```mermaid
flowchart TD
P[Parent or worker failure]-->M[Load run manifest]
M-->Q[Query WebCodex projects jobs and Git]
Q-->D{Operation exists?}
D-->|yes|R[Resume or observe same operation]
D-->|proven absent|N[Explicit replacement work]
R-->I[Continue]
N-->I
```

## Scheduler boundary
V1 does not use WebCodex AgentTask/Durable Agent as a second scheduler because that would duplicate Codex native task ownership. WebCodex durability is used for execution state and Jobs. AgentTask mapping is reserved for a V2 proposal after V1 acceptance.
