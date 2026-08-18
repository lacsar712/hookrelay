# HookRelay

对外 Webhook 可靠投递引擎：接收已签名的入站事件，按订阅规则扇出到多个目标 URL，带重试、熔断、限流、死信和人工重放。

本仓库是**可运行的健康项目**，不含预埋缺陷。

## 1. 为什么做这个而不是常见业务系统

这是一台**出站投递基础设施**，不是：

- 游戏 / 图形
- CLI 或桌面文件工具
- 电商、RBAC、库存、OA、医院挂号、预约、外卖、CRM、IM、工单、停车、拍卖
- 报表看板、记账、健身、菜谱、天气、番茄钟、习惯打卡、播放器、日记

产品边界：事件从调用方进来，经过签名校验与路由，由本进程负责**投递到别人的 HTTP 接口**。控制台只操作投递本身（目标、尝试记录、重放），不做统计分析产品。

## 2. 角色与主路径

| 角色 | 做什么 |
|------|--------|
| 调用方 | POST `/api/v1/events`，带 HMAC 签名与幂等键 |
| 投递目标 | 调用方登记的 HTTPS/HTTP URL，接收 JSON 回调 |
| 操作员 | 打开控制台登记目标、看最近投递、对失败事件点重放 |

健康路径：

1. 操作员登记一个目标（URL + 共享密钥 + 事件类型过滤器）。
2. 调用方推送一条事件（类型、载荷、幂等键）。
3. 引擎校验签名与时间窗，去重，匹配订阅，写入投递队列。
4. Worker 按退避策略 HTTP POST 到目标；成功记 journal；可重试失败进入下一轮；耗尽进入死信。
5. 操作员在页面看到尝试记录；失败项可重放。

## 3. 业务规则（必须实现，禁止空壳）

### 3.1 入站签名

- 算法：`HMAC-SHA256(secret, canonical)`，十六进制小写。
- Canonical 字符串：`v1.{timestamp}.{nonce}.{sha256_hex(raw_body)}`。
- Header：
  - `X-Hook-Timestamp`：Unix 秒
  - `X-Hook-Nonce`：16–64 字节可打印字符串
  - `X-Hook-Signature`：`v1=<hex>`
- 时间窗：默认 ±300 秒，超出拒绝（防重放）。
- 同一 `nonce` 在时间窗内只能成功一次。
- 入站密钥与目标出站密钥分开配置。

### 3.2 幂等

- 调用方必填 `Idempotency-Key`（8–128 字符）。
- 相同 key + 相同 body hash：返回第一次的受理结果，不重复入队。
- 相同 key + 不同 body hash：409 Conflict。
- 记录带 TTL（默认 24h），过期后允许复用 key。

### 3.3 订阅与扇出

- 目标（destination）包含：id、名称、URL、出站密钥、启用开关、事件类型前缀列表、每目标并发、每秒令牌桶。
- 事件 `type` 命中任一前缀（`order.` 匹配 `order.paid`）才投递。
- 一条入站事件可扇出到多个目标，每个目标独立队列与独立尝试记录。
- 目标可配置 `ordered=true`：同一目标上按入队顺序串行投递。

### 3.4 出站请求

- 方法 POST，`Content-Type: application/json`。
- 出站签名与入站同算法，使用**目标**密钥。
- 额外 header：`X-Hook-Event-Id`、`X-Hook-Delivery-Id`、`X-Hook-Attempt`、`X-Hook-Destination`。
- 超时：默认 10s；禁止跟随跨主机 3xx（最多同主机 1 次 307/308）。
- 响应 2xx 视为成功。
- 可重试：408、429、500–599、网络错误、超时。
- 不可重试：400–407、409–428、430–499（含 422），直接死信。

### 3.5 重试

- 满抖动指数退避：`sleep = random(0, min(cap, base * 2^attempt))`。
- 默认 `base=200ms`，`cap=30s`，`maxAttempts=8`（含首次）。
- 尝试序号从 1 开始写入 journal。

### 3.6 熔断

每个目标独立熔断器：

- Closed：连续失败 ≥ `failThreshold`（默认 5）→ Open。
- Open：拒绝新尝试 `openFor`（默认 30s），到期 → HalfOpen。
- HalfOpen：允许 `probe`（默认 1）次探测；成功 → Closed 并清计数；失败 → 再 Open。

Open 期间任务留在队列，不记为一次“业务失败尝试”，但 journal 记 `skipped_open`。

### 3.7 限流

每目标令牌桶：容量 `burst`，速率 `rate` 令牌/秒。没令牌则延迟再试，不消耗 attempt。

### 3.8 死信与重放

- 不可重试终态或 attempt 耗尽 → DLQ。
- 重放：从 journal 取出原始 body 与目标，生成新的 `delivery_id`，attempt 从 1 重新计数，不受旧幂等键阻挡（重放走内部入口）。

### 3.9 载荷与红线

- Body 最大 256 KiB。
- 控制台列表对 `authorization` / `password` / `secret` / `token` 字段做掩码，存储仍保留原文供重放。
- 进程退出前把内存 store 快照到数据目录；启动时加载。

## 4. 模块划分（对应多文件，便于日后跨文件缺陷）

| 包 | 职责 |
|----|------|
| `cmd/hookrelay` | 进程入口、信号退出、组装依赖 |
| `internal/config` | 环境变量与默认值、校验 |
| `internal/clock` | 可注入时钟（测试用） |
| `internal/hashutil` | body hash、规范串 |
| `internal/sign` | HMAC 签发与校验 |
| `internal/idempotency` | 幂等记录与冲突 |
| `internal/nonce` | nonce 去重 |
| `internal/event` | 入站事件解析与校验 |
| `internal/destination` | 目标 CRUD、前缀匹配、密钥重叠窗 |
| `internal/route` | 扇出计划 |
| `internal/queue` | 每目标队列（有序/无序） |
| `internal/backoff` | 退避与抖动 |
| `internal/circuit` | 熔断状态机 |
| `internal/ratelimit` | 令牌桶 |
| `internal/classify` | HTTP/网络错误是否可重试 |
| `internal/deliver` | 出站 HTTP 客户端 |
| `internal/worker` | 取队列、过熔断/限流、调用 deliver、写 journal |
| `internal/journal` | 尝试记录 |
| `internal/dlq` | 死信 |
| `internal/replay` | 重放 |
| `internal/store` | 快照持久化 |
| `internal/httpapi` | JSON API |
| `internal/web` | 静态页托管 |
| `internal/redact` | 控制台掩码 |
| `internal/idgen` | ULID 风格 id |

测试文件与包同目录，**不计入**生产行数。

## 5. HTTP API

基础 URL：`http://127.0.0.1:8080`

### 控制面（无签名，本机演示）

- `GET /api/v1/meta` → 版本、go、队列深度
- `GET /api/v1/destinations`
- `POST /api/v1/destinations` body: `{name,url,secret,type_prefixes[],ordered,rate,burst,enabled}`
- `POST /api/v1/destinations/{id}/enable` `{enabled:bool}`
- `GET /api/v1/journal?destination_id=&limit=`
- `GET /api/v1/dlq`
- `POST /api/v1/replay/{delivery_id}`
- `GET /api/v1/healthz`

### 数据面

- `POST /api/v1/events`
  - Headers：签名三件套 + `Idempotency-Key` + `X-Hook-Source-Key`（演示：入站密钥 id，默认 `ingest`）
  - Body：`{"type":"order.paid","payload":{...}}`

### 内置回环目标（便于无外网演示）

进程在 `:8080` 提供 `POST /api/v1/loopback`：原样记录最近 50 条收到的回调，供控制台展示“投递成功”。默认种子一条目标指向该 URL。

## 6. 前端（静态 HTML + 原生 JS，无构建、无 npm）

`web/index.html` + `web/app.js` + `web/style.css`

页面区块：

1. 服务状态（health + 队列深度）
2. 目标列表：新增、启用/停用
3. 发送测试事件（类型 + JSON payload）
4. 最近 journal
5. DLQ + 重放按钮
6. 回环目标最近收到的回调

交互必须走真实 API，禁止写死成功假数据。

## 7. 数据流

```
Caller --sign--> /events --> parse/validate
                         --> nonce + idempotency
                         --> route.fanout
                         --> queue.enqueue (per dest)
worker --> ratelimit.allow --> circuit.allow --> deliver.POST
      --> journal.append
      --> success: ack
      --> retryable: backoff + requeue
      --> terminal: dlq
UI replay --> replay.Enqueue --> 同上 worker 路径
store.snapshot <-- ticker / shutdown
```

## 8. 运行

```text
set GOTOOLCHAIN=local
go test ./...
go run ./cmd/hookrelay
```

浏览器打开 `http://127.0.0.1:8080/`。

环境变量（均有默认值）：

| 变量 | 默认 |
|------|------|
| `HOOKRELAY_ADDR` | `:8080` |
| `HOOKRELAY_DATA_DIR` | `./data` |
| `HOOKRELAY_INGEST_SECRET` | `dev-ingest-secret` |
| `HOOKRELAY_WINDOW_SEC` | `300` |

## 9. 技术约束

- Go 模块路径：`github.com/lacsar712/hookrelay`
- `go.mod` 语言版本 `1.22`；运行用本机工具链（`GOTOOLCHAIN=local`）
- **禁止 CGO**；存储用纯 Go 文件快照
- 标准库为主；不要为演示引入大型框架
- 每个包有真实分支逻辑，禁止复制粘贴空函数凑行数
- 本阶段不写 Docker、不埋 bug、不建 gold/test 分支

## 10. 目录骨架

```text
cmd/hookrelay/main.go
internal/...（上表各包）
web/index.html
web/app.js
web/style.css
PROJECT.md
README.md
go.mod
```
