# gproxy

`gproxy` 是一个使用 Go 编写的高并发 TCP / UDP 端口转发工具，适合做本地端口转发、批量端口映射、UDP 服务转发，以及需要通过配置文件统一管理多条转发规则的场景。

它当前支持：

- TCP 端口转发
- UDP 端口转发
- 单端口转发
- 端口范围转发
- 范围到单端口映射
- 范围到范围的一对一映射
- 运行中重载配置
- 查询运行状态
- 优雅停止服务

## 为什么做这个项目

很多端口转发工具只支持 TCP，或者只适合临时手工执行一两条规则。`gproxy` 的目标更偏向“服务化运行”：

- 使用 YAML 配置文件统一管理规则
- 一次启动，长期运行
- 同时覆盖 TCP 和 UDP
- 支持端口范围，不需要手写几十上百条单端口规则
- 提供本地控制命令，不必靠重启进程管理服务

## 适用场景

### 1. 本地开发环境转发

把本机某个端口暴露给另一个服务：

```text
127.0.0.1:8080 -> 10.0.0.2:80
```

适合把测试流量转发到内网服务、容器服务或远端机器。

### 2. 游戏或实时业务的 UDP 转发

UDP 业务通常不适合使用只支持 TCP 的通用反向代理。`gproxy` 可以直接监听 UDP 端口，并按客户端地址维护会话，把返回流量正确送回原始客户端。

### 3. 批量端口映射

例如一组服务实例需要把一整段端口统一映射到另一段端口：

```text
0.0.0.0:10000-10099 -> 10.0.0.2:20000-20099
```

### 4. 多端口汇聚到单端口

例如多个监听端口统一打到一个后端 UDP/TCP 端口：

```text
0.0.0.0:30000-30010 -> 10.0.0.3:40000
```

## 功能特性

- 使用 YAML 配置文件管理转发规则
- 同时支持 `tcp` 和 `udp`
- 支持以下三类映射：
  - `host:port -> host:port`
  - `host:port-range -> host:port`
  - `host:port-range -> host:port-range`
- 通过本地 Unix Socket 控制面提供管理命令
- `reload` 采用增量更新，未变化的监听器会继续保留
- `status` 可输出当前运行汇总状态
- `stop` 采用优雅停机，先停止接收新流量，再等待已有连接和会话结束

## 当前实现边界

当前版本有这些明确边界：

- 控制面为本地 Unix Socket，不是 HTTP API
- `status` 当前只提供汇总信息，不显示逐条规则状态
- 不做七层代理，只做原始 TCP / UDP 转发
- 不提供权限控制、认证、ACL、限流等功能
- 不做自动文件监听重载，使用显式 `reload` 命令

## 项目结构

```text
cmd/gproxy              CLI 入口
internal/app            命令分发与运行时编排
internal/config         配置加载、校验、端口范围展开
internal/control        本地控制协议与客户端/服务端
internal/runtime        运行时快照、状态与增量应用
internal/proxy/tcp      TCP 转发实现
internal/proxy/udp      UDP 转发实现
example/gproxy.yaml     示例配置
```

## 环境要求

- Go 1.26 或更高版本
- Linux / Unix 环境
  - 当前控制面依赖本地 Unix Socket

## 快速开始

### 1. 启动服务

```bash
go run ./cmd/gproxy run -c example/gproxy.yaml
```

如果你已经构建了二进制，也可以：

```bash
./gproxy run -c example/gproxy.yaml
```

### 2. 查看状态

```bash
go run ./cmd/gproxy status -c example/gproxy.yaml
```

示例输出：

```text
state: running
socket: /tmp/gproxy.sock
rules: 14
tcp listeners: 8
udp listeners: 6
```

字段含义：

- `state`
  - 当前运行状态，通常是 `running` 或 `stopping`
- `socket`
  - 控制面 Unix Socket 路径
- `rules`
  - 当前已经展开后的实际转发项数量
- `tcp listeners`
  - 当前活跃的 TCP 监听器数量
- `udp listeners`
  - 当前活跃的 UDP 监听器数量

### 3. 重载配置

修改配置文件后执行：

```bash
go run ./cmd/gproxy reload -c example/gproxy.yaml
```

### 4. 优雅停止

```bash
go run ./cmd/gproxy stop -c example/gproxy.yaml
```

## 配置文件格式

配置文件使用 YAML，包含两个部分：

- `control`
  - 控制面配置
- `rules`
  - 转发规则列表

示例文件见 [example/gproxy.yaml](/mnt/d/ideaprojects/gproxy/example/gproxy.yaml)。

完整示例：

```yaml
control:
  socket: /tmp/gproxy.sock
  udp_session_idle_timeout: 30s

rules:
  - name: tcp-web
    protocol: tcp
    listen: 0.0.0.0:8080
    target: 127.0.0.1:80

  - name: udp-range-fan-in
    protocol: udp
    listen: 0.0.0.0:30000-30010
    target: 127.0.0.1:40000

  - name: tcp-range-pair
    protocol: tcp
    listen: 0.0.0.0:10000-10002
    target: 127.0.0.1:20000-20002
```

## 配置字段说明

### `control`

#### `socket`

本地控制 Socket 路径，例如：

```yaml
control:
  socket: /tmp/gproxy.sock
```

`reload`、`status`、`stop` 都通过它和运行中的 `gproxy` 进程通信。

#### `udp_session_idle_timeout`

UDP 会话空闲超时时间，例如：

```yaml
control:
  udp_session_idle_timeout: 30s
```

用途有两个：

- 控制 UDP 会话空闲多久后被回收
- 影响 `stop` 时的优雅等待窗口

### `rules`

每条规则包含四个核心字段：

- `name`
  - 规则名称
- `protocol`
  - 协议类型，只支持 `tcp` 或 `udp`
- `listen`
  - 本地监听地址
- `target`
  - 目标转发地址

示例：

```yaml
- name: tcp-web
  protocol: tcp
  listen: 0.0.0.0:8080
  target: 127.0.0.1:80
```

## 地址格式

`listen` 和 `target` 支持两种格式：

- `host:port`
- `host:start-end`

例如：

- `127.0.0.1:8080`
- `0.0.0.0:10000-10010`

当前要求 `host` 必须显式填写，不支持只写端口。

## 支持的映射语义

### 1. 单端口到单端口

```text
127.0.0.1:8080 -> 10.0.0.2:80
```

这是最常见的场景，一条监听端口对应一个目标端口。

### 2. 端口范围到单端口

```text
0.0.0.0:30000-30010 -> 10.0.0.3:40000
```

表示监听 `30000` 到 `30010` 的所有端口，并统一转发到目标 `40000`。

适合“多入口，单后端端口”的场景。

### 3. 端口范围到端口范围

```text
0.0.0.0:10000-10002 -> 10.0.0.2:20000-20002
```

会按偏移量一对一映射：

- `10000 -> 20000`
- `10001 -> 20001`
- `10002 -> 20002`

这种映射要求源范围和目标范围长度一致，否则配置校验会失败。

## 常见配置示例

### 示例 1：单条 TCP 转发

```yaml
control:
  socket: /tmp/gproxy.sock
  udp_session_idle_timeout: 30s

rules:
  - name: web
    protocol: tcp
    listen: 0.0.0.0:8080
    target: 127.0.0.1:80
```

### 示例 2：单条 UDP 转发

```yaml
control:
  socket: /tmp/gproxy.sock
  udp_session_idle_timeout: 30s

rules:
  - name: dns
    protocol: udp
    listen: 0.0.0.0:5353
    target: 8.8.8.8:53
```

### 示例 3：端口范围汇聚到单端口

```yaml
control:
  socket: /tmp/gproxy.sock
  udp_session_idle_timeout: 30s

rules:
  - name: udp-fan-in
    protocol: udp
    listen: 0.0.0.0:30000-30010
    target: 10.0.0.3:40000
```

### 示例 4：端口范围一对一映射

```yaml
control:
  socket: /tmp/gproxy.sock
  udp_session_idle_timeout: 30s

rules:
  - name: tcp-pair
    protocol: tcp
    listen: 0.0.0.0:10000-10010
    target: 10.0.0.2:20000-20010
```

### 示例 5：同时使用 TCP 和 UDP

```yaml
control:
  socket: /tmp/gproxy.sock
  udp_session_idle_timeout: 30s

rules:
  - name: tcp-api
    protocol: tcp
    listen: 0.0.0.0:8080
    target: 127.0.0.1:18080

  - name: udp-game
    protocol: udp
    listen: 0.0.0.0:9000-9005
    target: 127.0.0.1:10000-10005
```

## 管理命令说明

### `run`

启动代理服务：

```bash
go run ./cmd/gproxy run -c example/gproxy.yaml
```

### `reload`

重载配置：

```bash
go run ./cmd/gproxy reload -c example/gproxy.yaml
```

行为说明：

- 新增规则会启动新监听器
- 删除规则会关闭旧监听器
- 未变化规则保持不动
- 已建立的 TCP 连接会继续转发直到自然结束
- UDP 已建立会话会按会话生命周期自然回收

### `status`

查看当前运行状态：

```bash
go run ./cmd/gproxy status -c example/gproxy.yaml
```

当前输出为汇总级别，不显示每条规则的明细。

### `stop`

优雅停止运行中的服务：

```bash
go run ./cmd/gproxy stop -c example/gproxy.yaml
```

行为说明：

- 先停止接收新的 TCP 连接
- UDP 不再建立新的客户端会话
- 已存在流量尽量自然结束
- 达到优雅等待窗口后统一收尾退出

## 工作机制

### TCP 转发

每个展开后的 TCP 转发项会创建一个独立监听器：

1. 接收客户端连接
2. 连接目标地址
3. 建立双向数据拷贝
4. 连接结束后回收资源

### UDP 转发

UDP 是无连接协议，所以 `gproxy` 会为每个客户端地址维护会话：

1. 收到客户端 UDP 包
2. 按客户端源地址查找或创建会话
3. 将数据发往目标地址
4. 目标返回数据时，再写回原客户端
5. 会话空闲超时后自动清理

## 配置校验规则

启动和重载前，程序会对配置做校验。以下情况会直接报错：

- `protocol` 不是 `tcp` 或 `udp`
- 地址格式非法
- 端口超出 `1-65535`
- 范围起始端口大于结束端口
- 源是单端口，目标却是端口范围
- 源范围和目标范围长度不一致
- 展开后监听地址冲突
- 规则名称重复

## 构建

构建二进制：

```bash
go build -o gproxy ./cmd/gproxy
```

运行二进制：

```bash
./gproxy run -c example/gproxy.yaml
```

## 测试

运行全部测试：

```bash
go test ./... -v
```

## 性能与实现取向

当前实现偏向：

- 高并发稳定性
- 低额外抽象开销
- 配置展开后走简单数据路径
- 增量重载，减少对现有连接的影响

它不是一个七层网关，也不试图提供复杂流量治理能力。

## 未来可扩展方向

如果后续继续演进，比较自然的方向有：

- 更细粒度的状态输出
- `status` 支持 JSON
- 控制面增加 `stop --force`
- 指标导出
- ACL / 限流 / 访问控制
