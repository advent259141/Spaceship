# spaceship

`spaceship` 是面向 AstrBot 的远程节点控制方案仓库。

`agent/`：Go 编写的跨平台节点客户端，负责连接 AstrBot 网关并执行远端任务

当前 AstrBot 侧的正式实现位于本体仓库中的 `astrbot/core/spaceship/`。

## 当前目标

Spaceship 的目标是把 AstrBot 扩展成一个远端节点网关：

- 远端机器运行 Go agent 并与 AstrBot 建立 WebSocket 长连接
- AstrBot 作为控制面向在线节点派发任务
- LLM 可以通过工具查看节点、执行命令、列目录、读取文件、写入文件
- WebUI 可以管理 Spaceship 配置并查看节点状态

## 当前架构

### AstrBot 本体侧

主实现已经落在 AstrBot core 子模块：

- `astrbot/core/spaceship/models.py`
- `astrbot/core/spaceship/session.py`
- `astrbot/core/spaceship/dispatcher.py`
- `astrbot/core/spaceship/gateway.py`
- `astrbot/core/spaceship/tools.py`
- `astrbot/core/spaceship/tool_registry.py`
- `astrbot/core/spaceship/components.py`
- `astrbot/core/spaceship/runtime.py`

配套接入点包括：

- `astrbot/core/core_lifecycle.py`
- `astrbot/dashboard/routes/spaceship.py`
- `astrbot/core/config/default.py`
- dashboard 前端页面、路由、侧边栏和 i18n

### Go agent 侧

Go agent 当前负责：

- 读取 `.env` 或环境变量配置
- 与 AstrBot 网关建立 WebSocket 连接
- 发送 `node.hello`
- 接收 `node.welcome`
- 定时发送 `node.heartbeat`
- 接收 `task.dispatch`
- 执行 `exec`、`list_dir`、`read_file`、`write_file`、`edit_file`、`grep`
- 执行 `delete_file`、`move_file`、`copy_file`
- 检测系统 Python 并自动创建/复用 venv，支持 `exec_python`
- 回传 `task.accepted`、`task.started`、`task.output`、`task.result`
- 在连接断开时自动退避重连

## 目录说明

```text
.
├─ README.md
├─ plans/
│  ├─ spaceship-astrbot-architecture.md
│  └─ spaceship-current-status.md
├─ agent/
│  ├─ .env
│  ├─ go.mod
│  ├─ go.sum
│  ├─ cmd/
│  │  └─ spaceship-agent/
│  │     └─ main.go
│  └─ internal/
│     ├─ config/
│     ├─ executor/
│     ├─ fileops/
│     ├─ heartbeat/
│     ├─ logger/
│     ├─ metadata/
│     ├─ policy/
│     ├─ protocol/
│     ├─ python/
│     ├─ registrar/
│     ├─ shell/
│     └─ wsclient/
└─ plugin/
   └─ spaceship/
      └─ ... (legacy prototype)
```

## 已完成能力

当前已经落地：

1. AstrBot core 子模块化 `spaceship`
2. WebUI 配置页面、节点列表与节点详情接口
3. WebSocket 网关接入路径 `/api/spaceship/ws`
4. LLM 工具注册：`listnode`、`getnodeinfo`、`executeshell`、`listdir`、`readfile`、`writefile`
5. Go agent 的 `.env` 配置加载
6. Go agent 的基本日志输出
7. Go agent 的自动重连与退避重试参数
8. 最小联调闭环：hello / welcome / heartbeat / task.dispatch / task.result

## 当前可用工具

当前已经接入 AstrBot 的 LLM 工具有：

- `listnode`：查看当前在线或指定状态的节点列表
- `getnodeinfo`：查看指定节点的详细信息
- `executeshell`：在指定节点上执行 shell 命令
- `listdir`：查看指定节点上的目录内容
- `readfile`：读取指定节点上的文本文件内容
- `writefile`：向指定节点上的文件写入文本内容，支持覆盖或追加
- `editfile`：通过搜索替换编辑指定节点上的文件
- `grepfile`：在指定节点上搜索文件内容
- `deletefile`：删除指定节点上的文件或目录
- `movefile`：移动或重命名指定节点上的文件或目录
- `copyfile`：复制指定节点上的文件或目录
- `executepython`：在指定节点上执行 Python 代码（需节点安装 Python）

## Go agent 配置

agent 支持三种配置方式，加载优先级为：

**CLI flags > 环境变量 > YAML 配置文件 > 内置默认值**

### 方式一：YAML 配置文件（推荐）

将 `spaceship.yaml.example` 复制为 `spaceship.yaml`：

```yaml
server_url: ws://127.0.0.1:6185/api/spaceship/ws
node_id: dev-node-01
token: change-me
alias: Local Dev Node

log_level: info
heartbeat_interval: 20s

reconnect:
  min_delay: 1s
  max_delay: 30s

python:
  path: ""           # 留空则自动检测
  skip_venv: false
```

agent 自动按以下顺序搜索配置文件：
1. `--config` 命令行参数指定的路径
2. `SPACESHIP_CONFIG_FILE` 环境变量
3. 工作目录下的 `spaceship.yaml` / `spaceship.yml`
4. 可执行文件同目录下的 `spaceship.yaml` / `spaceship.yml`

### 方式二：CLI Flags

```powershell
spaceship-agent.exe --server ws://192.168.1.100:6185/api/spaceship/ws --node-id my-node --token secret
```

可用 flags：

| Flag | 说明 |
|------|------|
| `--config` | 指定 YAML 配置文件路径 |
| `--server` | AstrBot WebSocket 网关地址 |
| `--node-id` | 节点唯一 ID |
| `--token` | 认证 token |
| `--alias` | 节点显示名称 |
| `--log-level` | 日志级别 (debug/info/warn/error) |

### 方式三：环境变量 / .env 文件

agent 目录下的 `.env` 文件仍然支持（向后兼容）：

```env
SPACESHIP_SERVER_URL=ws://127.0.0.1:6185/api/spaceship/ws
SPACESHIP_NODE_ID=dev-node-01
SPACESHIP_NODE_ALIAS=Local Dev Node
SPACESHIP_NODE_TOKEN=change-me
SPACESHIP_LOG_LEVEL=info
SPACESHIP_HEARTBEAT_INTERVAL=20s
SPACESHIP_RECONNECT_MIN_DELAY=1s
SPACESHIP_RECONNECT_MAX_DELAY=30s

# Python 配置（可选）
# SPACESHIP_PYTHON_PATH=
# SPACESHIP_SKIP_PYTHON_VENV=false
```

## 运行方式

### Windows 本地运行

```powershell
Set-Location .\agent
go run .\cmd\spaceship-agent
```

### 编译后运行

```powershell
Set-Location .\agent
go build -trimpath -ldflags="-s -w" -o spaceship-agent.exe .\cmd\spaceship-agent
.\spaceship-agent.exe
```

### 编译 Linux ELF

```powershell
Set-Location .\agent
$env:GOOS="linux"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"
go build -trimpath -ldflags="-s -w" -o spaceship-agent .\cmd\spaceship-agent
```

## 当前状态

当前项目已经进入联调清障阶段，而不是纯方案阶段。

已经稳定完成的部分：

- 配置持久化
- dashboard 路由接入
- core 工具注册
- WebSocket 上下文问题修复
- 基础节点能力和远端文件/命令调用链路

仍在继续完善的部分：

- `task.cancel`
- 更细粒度权限控制
- 输出分块优化
- 审计落库
- 更完整的连接恢复与任务恢复策略


