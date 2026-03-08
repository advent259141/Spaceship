# spaceship

`spaceship` 是面向 AstrBot 的远程节点控制方案原型仓库。

当前仓库按两部分拆分：

- `agent/`：Go 编写的跨平台节点客户端，首期支持 Linux 与 Windows
- `plugin/`：AstrBot 侧的 `spaceship` 插件骨架，负责节点管理、任务调度、工具路由与审计
- `plans/`：架构设计文档，当前主文档为 `plans/spaceship-astrbot-architecture.md`

## 首期技术选型

### Go agent
- Go `1.23`
- WebSocket：`github.com/gorilla/websocket`
- 配置：标准库 `encoding/json`
- 日志：标准库 `log/slog`
- 命令执行：标准库 `os/exec`

### AstrBot plugin
插件骨架按 Python 目录结构组织，目标是表达控制面模块边界，并逐步对齐 AstrBot 的 computer layer：

- `manifest`：插件元信息占位
- `models`：节点、任务、协议、工具请求模型
- `storage`：SQLite 存储接口
- `session_hub`：WebSocket 会话管理
- `node_registry`：节点注册与在线状态
- `dispatcher`：任务分发与结果聚合
- `runtime_adapter`：`SpaceshipNodeBooter` 与远端 shell/fs 组件
- `booter_factory`：运行时 booter 选择入口
- `tool_adapter`：AstrBot 工具适配层
- `policy_engine`：权限与路径策略
- `audit`：任务审计与安全审计

## 目录规划

```text
.
├─ README.md
├─ plans/
│  └─ spaceship-astrbot-architecture.md
├─ agent/
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
│     ├─ registrar/
│     ├─ shell/
│     └─ wsclient/
└─ plugin/
   └─ spaceship/
      ├─ app.py
      ├─ README.md
      ├─ manifest.json
      ├─ __init__.py
      ├─ audit/
      ├─ booter_factory/
      ├─ dispatcher/
      ├─ models/
      ├─ node_registry/
      ├─ policy_engine/
      ├─ runtime_adapter/
      ├─ session_hub/
      ├─ storage/
      └─ tool_adapter/
```

## 当前落地范围

当前已经落地：

1. Go agent 的协议模型、配置、WebSocket 客户端骨架与 shell 执行器
2. AstrBot 插件的目录结构、核心服务、`SpaceshipNodeBooter` 与统一 runtime 入口
3. 节点 hello、welcome、heartbeat、task.dispatch、task.output、task.result 的最小事件闭环
4. 最小可扩展的数据模型与说明文档

## 下一步

1. 在 plugin 侧接入真实 WebSocket 路由或 AstrBot 生命周期入口
2. 补齐 `task.cancel`、文件读取与更细粒度输出分块
3. 对齐 AstrBot `shell.exec(...)` 与 `fs.read_file(...)` 的返回语义
4. 补齐 SQLite 持久化与审计落库
5. 接入 AstrBot 实际工具注册与节点管理接口
