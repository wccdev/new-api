# chatlog 运维手册

chatlog 是本 fork 独有的功能：完整记录对话类中继请求的「请求 + 响应」，用于
后续微调训练，并在「使用日志」页面每行提供「对话」按钮查看完整对话。

- 默认关闭；不设置 `CHATLOG_ENABLED=true` 时行为与上游完全一致。
- 录制范围：`POST` + `application/json` 的
  `/v1/chat/completions`、`/v1/completions`、`/v1/responses`、`/v1/messages`、
  `/v1beta/models/*:generateContent|streamGenerateContent`、`/pg/chat/completions`，
  流式与非流式都录。WebSocket（realtime）、multipart（音频/图片上传）不录。
- 不保存 API Key / Authorization 头。
- 录制是异步的：任何录制错误都只写系统日志，不影响转发。

## 1. 部署与环境变量

### 1.1 相比上游镜像需要改什么

只有三件事，两个节点都要做：

1. 镜像换成 `aidenlu/new-api`（生产建议用不可变标签，见第 5 节）。
2. **新增**环境变量 `CHATLOG_ENABLED=true` 和 `CHATLOG_NODE=<节点名>`（其余 `CHATLOG_*` 都有默认值，按需设置）。
3. 确认 `/data` 挂载到了宿主机卷（上游默认 compose 已有 `./data:/data`），否则归档文件会随容器重建丢失。

数据库、Redis、`SESSION_SECRET`、`CRYPTO_SECRET`、`NODE_TYPE` 等原有配置保持不变，chatlog 不需要新的外部依赖。

### 1.2 新增的环境变量

| 环境变量 | 默认值 | 是否必配 | 说明 |
|---|---|---|---|
| `CHATLOG_ENABLED` | `false` | **必配** | 总开关。不设或为 `false` 时行为与上游完全一致 |
| `CHATLOG_NODE` | 容器主机名 | **强烈建议** | 写进归档文件名和每条记录。容器主机名在重建后会变，所以要固定设置，且两个节点不同 |
| `CHATLOG_DIR` | `/data/chatlog` | 可选 | 归档文件目录，必须位于挂载到宿主机的卷内 |
| `CHATLOG_MAX_REQ_MB` | `16` | 可选 | 单条请求体上限（MB），超出截断并标记 `truncated` |
| `CHATLOG_MAX_RESP_MB` | `8` | 可选 | 单条响应体上限（MB），同上 |
| `CHATLOG_DB_RETENTION_DAYS` | `30` | 可选 | 数据库里"界面查看用副本"的保留天数，`0` = 永久。只在主节点执行清理 |
| `CHATLOG_FILE_RETENTION_DAYS` | `0` | 可选 | 归档文件保留天数，`0` = 永久。每个节点清理自己的目录 |
| `CHATLOG_QUEUE_SIZE` | `256` | 可选 | 异步写入队列长度，满了丢弃录制并在系统日志计数 |

所有变量只在启动时读取，修改后需要重启容器。

### 1.3 两节点 compose 示例

主节点（node-1）：

```yaml
services:
  new-api:
    image: aidenlu/new-api:latest          # 生产建议固定为 aidenlu/new-api:<上游tag>-chatlog-<commit8位>
    restart: always
    command: --log-dir /app/logs
    ports:
      - "3000:3000"
    volumes:
      - ./data:/data                       # 归档文件在宿主机 ./data/chatlog
      - ./logs:/app/logs
    environment:
      # ---- 原有配置，保持你现在的值 ----
      - SQL_DSN=...                        # 两个节点指向同一个数据库
      - REDIS_CONN_STRING=...              # 两个节点指向同一个 Redis
      - SESSION_SECRET=...                 # 两个节点相同
      - CRYPTO_SECRET=...                  # 两个节点相同
      - TZ=Asia/Shanghai
      # ---- chatlog 新增 ----
      - CHATLOG_ENABLED=true
      - CHATLOG_NODE=node-1
      # - CHATLOG_DB_RETENTION_DAYS=30
      # - CHATLOG_FILE_RETENTION_DAYS=0
      # - CHATLOG_MAX_REQ_MB=16
      # - CHATLOG_MAX_RESP_MB=8
```

从节点（node-2）只有两处不同：

```yaml
    environment:
      - NODE_TYPE=slave                    # 原有配置：从节点
      - CHATLOG_ENABLED=true
      - CHATLOG_NODE=node-2
```

### 1.4 上线步骤

1. 先改主节点：`docker compose pull && docker compose up -d`。
2. 看启动日志出现 `chatlog enabled, archive dir: /data/chatlog`（`docker compose logs new-api | grep chatlog`）。
   主节点此时会自动创建 `chat_records` 表。
3. 再用同样方式升级从节点。
4. 验证：发一条聊天请求 → 「使用日志」该行点「对话」能看到完整内容；宿主机
   `ls data/chatlog/dt=$(date +%F)/` 能看到 `HH-node-1.jsonl.zst`。

**升级顺序：先升级主节点**（`NODE_TYPE` 不是 `slave` 的那个）。建表只在主节点执行；
从节点先起来会在系统日志里报写库失败（归档文件不受影响），主节点起来后自动恢复。

## 2. 数据存在哪

同一条记录存两份，用途不同：

| | 归档文件（训练用） | 数据库副本（界面查看用） |
|---|---|---|
| 位置 | **各节点本地** `CHATLOG_DIR/dt=YYYY-MM-DD/HH-<节点名>.jsonl.zst`，容器内 `/data/chatlog`，按默认 compose 即宿主机 `./data/chatlog` | 共享数据库的 `chat_records` 表（配置了 `LOG_SQL_DSN` 就在日志库；日志库是 ClickHouse 时在主库） |
| 内容 | 每行一条 JSON 记录，每条记录是一个独立 zstd 帧，整个文件是合法的 `.jsonl.zst` | `payload` 列 = 同一条记录的 zstd 压缩字节 |
| 保留 | 默认永久 | 默认 30 天，主节点每小时清理 |
| 谁读 | `chatlog-export`、你自己的脚本 | `GET /api/chatlog/:request_id`（管理员）、`/api/chatlog/self/:request_id`（本人） |

两个节点各写各的文件，文件名带节点名，拷到同一个目录不会冲突。

### 记录格式（`schema: 1`）

```json
{
  "schema": 1, "request_id": "2026...", "created_at": 1789979483, "latency_ms": 8300,
  "protocol": "openai.chat", "path": "/v1/chat/completions", "stream": true, "status": 200,
  "user_id": 1, "username": "alice", "token_id": 3, "token_name": "dev", "group": "default",
  "channel_id": 7, "model": "gpt-4o", "client_ip": "10.0.0.8", "user_agent": "...", "node": "node-1",
  "truncated": false,
  "request":  { "...客户端原始请求体，原样..." },
  "response": { "...客户端收到的完整响应；流式会被重组成等价的非流式对象..." }
}
```

- `protocol`：`openai.chat` / `openai.completions` / `openai.responses` /
  `anthropic.messages` / `gemini.generate`。
- 请求体不是合法 JSON（被截断）时用 `request_raw`（字符串）代替 `request`；
  流式响应无法重组（被截断、客户端中途断开且无有效数据）时用 `response_raw` 保存原始 SSE 文本。
- `request_id` 与「使用日志」里的 Request ID、响应头 `X-Oneapi-Request-Id` 一致。

### 直接查看归档文件

```bash
zstd -dc data/chatlog/dt=2026-09-21/13-node-1.jsonl.zst | head -1 | jq .
zstd -dc data/chatlog/dt=2026-09-21/*.jsonl.zst | jq -r '[.request_id,.model,.username,.status] | @tsv'
```

```python
import io, json, zstandard
with open("13-node-1.jsonl.zst", "rb") as f:
    for line in io.TextIOWrapper(zstandard.ZstdDecompressor().stream_reader(f), encoding="utf-8"):
        record = json.loads(line)
```

## 3. 导出训练集

### 3.1 汇总两个节点的归档

```bash
mkdir -p /srv/chatlog-all
rsync -av node1:/path/to/new-api/data/chatlog/ /srv/chatlog-all/
rsync -av node2:/path/to/new-api/data/chatlog/ /srv/chatlog-all/
```

当前小时的文件还在被追加写入，拷走也能正常解压（每条记录是完整的帧），只是不含之后的记录；
要完整数据就只导出到昨天（`-to`）。

### 3.2 运行导出工具

仓库里带了预编译好的 linux/amd64 二进制 `chatlog/bin/chatlog-export-linux-amd64`（静态链接，无依赖，
拷到任意 amd64 Linux 机器即可运行）：

```bash
chmod +x chatlog-export-linux-amd64
./chatlog-export-linux-amd64 -dir /srv/chatlog-all -out sft.jsonl -from 2026-09-01 -to 2026-09-30
```

导出逻辑（`chatlog/cmd/chatlog-export`、`chatlog/convert`）有改动后需要重新编译并提交这个二进制：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" \
  -o chatlog/bin/chatlog-export-linux-amd64 ./chatlog/cmd/chatlog-export
```

其他平台去掉 `GOOS/GOARCH` 自行编译即可（需要 Go，版本见 `go.mod`）。

| 参数 | 说明 |
|---|---|
| `-dir` | 归档目录（含 `dt=*` 子目录），默认 `/data/chatlog` |
| `-out` | 输出文件，默认 `sft.jsonl` |
| `-from` / `-to` | 起止日期 `YYYY-MM-DD`（含），按目录名过滤 |
| `-model` | 逗号分隔的子串，模型名包含任一即保留，如 `-model gpt-4o,claude` |
| `-user` | 只导出某个用户 id |
| `-reasoning` | 把思考过程作为 `reasoning_content` 一并导出（默认不导出） |
| `-metadata` | 每行附带 `metadata`（request_id、model、user_id、protocol、时间），便于回溯 |
| `-keep-prefixes` | 关闭多轮去重（见下） |

结束时会在 stderr 打印 `usable records: N, written: M`。

### 3.3 输出格式

OpenAI chat 微调格式，一行一个样本，四种协议统一转换成这一种：

```json
{"messages":[{"role":"system","content":"..."},{"role":"user","content":"..."},
             {"role":"assistant","content":"...","tool_calls":[{"id":"call_1","type":"function","function":{"name":"f","arguments":"{...}"}}]},
             {"role":"tool","tool_call_id":"call_1","content":"..."},
             {"role":"assistant","content":"..."}],
 "tools":[{"type":"function","function":{"name":"f","description":"...","parameters":{}}}]}
```

可直接用于 OpenAI / Azure 微调、TRL `SFTTrainer`、ms-swift、Unsloth、Axolotl（`type: chat_template`）；
LLaMA-Factory 用 `formatting: sharegpt`，并在 `dataset_info.json` 的 `tags` 里把
`role_tag/content_tag/user_tag/assistant_tag` 设为 `role/content/user/assistant`。

### 3.4 导出时自动做的筛选

- 只要 `status == 200`、未截断、请求响应都是合法 JSON 的记录。
- 只要正常结束的生成：OpenAI `finish_reason ∈ {stop, tool_calls, function_call}`；
  Responses `status == completed`；Claude `stop_reason ∈ {end_turn, tool_use, stop_sequence}`；
  Gemini `finishReason == STOP`。被长度截断、内容过滤、报错的都丢弃。
- **多轮去前缀**：聊天接口无状态，10 轮对话会产生 10 条互为前缀的记录。工具按
  「同一用户 + 请求消息序列」识别，只保留每段对话最长的那条；完全相同的请求只保留第一条。
  系统提示词里带动态内容（如当前时间）的客户端会打断这个识别，属已知限制。
- 只导出文本；图片不进训练集（原始归档里有）。

脱敏、质量打分、近似去重（MinHash）等不在工具内，按需在 `sft.jsonl` 上另做。

## 4. 容量与备份

- Agent 类流量（Claude Code、Cursor）每次请求都带全量历史，单条几十到几百 KB，zstd 后约 1/5–1/10。
  上线后观察几天 `du -sh data/chatlog/dt=*` 再定保留策略。
- 归档文件是训练数据的唯一长期来源（数据库副本默认只留 30 天），建议每天把昨天的目录同步到
  对象存储 / NAS：`rsync -av data/chatlog/dt=$(date -d yesterday +%F) backup:/chatlog/`。
- 磁盘注意：new-api 自带过载保护，磁盘使用率超过阈值（默认 95%，系统设置 → 性能）会让中继返回 503
  `system_disk_overloaded`。归档目录和系统盘同盘时要留意。
- 数据库：`chat_records.payload` 在 MySQL 是 `longblob`、PostgreSQL 是 `bytea`。想让界面能看更久就调大
  `CHATLOG_DB_RETENTION_DAYS`，代价是库变大。

## 5. 镜像构建与升级（GitHub Actions）

`.github/workflows/fork-sync-build.yml`：

- 触发：每天 03:17（北京时间）；`main` 上 chatlog 相关文件有推送；Actions 页面手动 **Run workflow**
  （可填 `upstream_tag` 指定上游 tag，勾 `force_build` 强制重建）。
- 流程：取上游最新 **release** tag → merge 进 `main`（不 rebase、不 force-push）→ 检查三处挂载点还在、
  `go vet`、`go test ./chatlog/...` → 推送 `main` → 构建并推送 `linux/amd64` 镜像。
- 镜像标签：`aidenlu/new-api:<上游tag>-chatlog-<commit8位>`（不可变）、`:<上游tag>-chatlog`、`:latest`。
  生产建议固定用不可变标签，回滚就是换回上一个标签。
- 需要的 secrets：`SYNC_PAT`（classic PAT，`repo` + `workflow`）、`DOCKER_USERNAME`、`DOCKER_PASSWORD`；
  可选变量 `DOCKERHUB_IMAGE`。PAT 过期后 workflow 会在 checkout/push 处认证失败，重新生成并更新 secret。
- **不要往本 fork 推 git tag**，上游自带的按 tag 触发的 workflow 应保持禁用。
- **合并冲突**：workflow 失败并自动开 issue 列出冲突文件。本功能对上游文件只有三处改动，手动处理：

  ```bash
  git fetch https://github.com/QuantumNous/new-api.git tag <上游tag>
  git checkout main && git merge <上游tag>
  # 保证下面三处还在，其余一律以上游为准：
  #   router/relay-router.go   router.Use(middleware.ChatlogCapture())  —— 紧跟 BodyStorageCleanup() 之后
  #   router/main.go           SetChatlogRouter(router)
  #   web/src/features/usage-logs/components/columns/common-logs-columns.tsx  ConversationButton 那一列
  go test ./chatlog/... && git commit && git push origin main
  ```

## 6. 排障

| 现象 | 排查 |
|---|---|
| 点「对话」显示"无对话记录" | 该请求发生在启用之前；或已超过 `CHATLOG_DB_RETENTION_DAYS`；或不在录制范围（embeddings、图片、音频、realtime）；或鉴权失败的请求（不录）。 |
| 启动日志没有 `chatlog enabled, archive dir: ...` | 该节点没设 `CHATLOG_ENABLED=true`，或紧邻上方有 `chatlog disabled ...` 的报错（建表失败等）。 |
| 系统日志 `chatlog: failed to persist record ...` | 写文件失败（目录没挂载/无权限/磁盘满）或写库失败（从节点先于主节点升级导致表不存在；MySQL 单条超过 `max_allowed_packet`，5.7 默认 4MB，可调大）。文件和库是独立写的，一边失败不影响另一边。 |
| 系统日志 `chatlog queue full, records dropped so far: N` | 写入跟不上（通常是数据库慢）。调大 `CHATLOG_QUEUE_SIZE`，检查数据库延迟。丢的只是录制，不影响请求。 |
| 普通用户看不到别人的记录 | 设计如此：`/self` 接口对"不存在"和"不是你的"返回相同结果。 |
| 对话页里助手消息的 XML 标签没显示 | 助手消息走项目的 Markdown 渲染，会过滤未知标签；「原始 JSON」页是完整内容。 |
| 手机端没有「对话」按钮 | 已知限制，手机卡片视图未接入。 |

关闭功能：去掉 `CHATLOG_ENABLED` 重启即可；已有文件和 `chat_records` 表保留，不会被删除。
彻底清理：删除 `CHATLOG_DIR` 目录，`DROP TABLE chat_records;`。
