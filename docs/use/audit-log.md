# 输出审计（HTTP / SSE）

在「设置 → 运营设置 → 输出审计」选择渠道、用户 API Key，点击「保存审计配置」。两项都清空或点击「关闭审计」即可关闭。这里只保存渠道 ID 和用户令牌 ID，不保存明文 API Key。

| 渠道 | 用户 API Key | 匹配范围 |
| --- | --- | --- |
| 空 | 空 | 关闭（默认） |
| 非空 | 空 | 命中任一配置渠道 |
| 空 | 非空 | 命中任一配置令牌 |
| 非空 | 非空 | 同时命中渠道和令牌（交集） |

保存后当前实例立即对新请求生效；其他共享数据库实例每 5 秒重新加载一次，不依赖 `MEMORY_CACHE_ENABLED`。在途请求沿用进入时的配置，已入队事件仍可能继续写入。首次部署此功能需要正常更新程序；之后修改审计配置无需重启进程或 Docker。

配置复用 RootAuth 管理接口 `PUT /api/option/`，例如：

```json
{
  "key": "AuditLogConfig",
  "value": "{\"channel_ids\":[25],\"token_ids\":[7]}"
}
```

`channel_ids` 和 `token_ids` 必须是正整数数组。`token_ids` 指调用 Done Hub 的用户令牌 ID，不是渠道凭据 ID。非法配置或数据库保存失败时保留原配置。后台同步失败时也保留最后一次有效配置。

`archive_enabled` 是独立的文件管理开关，默认 `false`；旧配置省略该字段时继续使用普通模式。它不改变筛选语义，也不会单独启用审计。页面的「归档模式」随「保存审计配置」热生效。

## 文件与还原

普通模式文件位于 `<log_dir>/audit/audit.log`；默认 Docker 镜像工作目录为 `/data`，所以默认是 `/data/logs/audit/audit.log`，现有 Compose 对应宿主机 `./data/logs/audit/audit.log`。默认关闭且没有遗留归档时不创建文件。目录以 `0700`、文件以 `0600` 创建，审计正文不写入系统日志或 stderr。

每个实例应使用独立日志目录，不要让多个进程轮转同一个文件。单文件上限 100 MB，最多 10 个备份，备份保留 7 天。轮转不是独立磁盘配额；共享磁盘仍需按常规方式监控可用空间。

归档模式使用独立的 `<log_dir>/audit/archive/`，文件生命周期和宿主机备份说明见 [审计归档与备份](audit-backup.md)。普通模式的 7 天 / 10 份策略不作用于该目录；切回普通模式也不会删除待归档文件。

格式为 JSONL，每行一个事件：

- `upstream_start`：一次实际上游 HTTP 调用开始。
- `response_start` / `response_body` / `response_end`：上游响应头、原始字节块和读取终态。
- `request_start` / `request_body` / `request_end`：入站原始请求，在 handler 结束时提交。

按 `request_id` 关联客户端请求；响应再按 `attempt`、`channel_id` 和 `exchange` 区分重试及同一 attempt 内的多个 HTTP 调用。请求事件可能出现在响应事件之后，这是为等待渠道匹配并保持请求线程不等待文件 I/O。

`data` 字段是 Base64；按事件顺序和 `seq` 拼接解码即可恢复原始 Body（含 SSE 空白、换行、二进制内容）。请求在参数映射、模型映射和协议转换前观察；响应在解析和转换前观察。HTTP 标准解压、分块解码可能已经由 `net/http` 完成，此处不是 TLS/TCP 报文捕获。

headers 仅保留 Content-Type、Content-Encoding、Accept、User-Agent 及请求 ID 等运行信息；Authorization、Cookie、签名及自定义 headers 不保留，URL 查询中的鉴权字段脱敏。Body 按原文字节记录，可能含用户输入和上游生成内容。

## 隔离与完整性

审计只复制业务已经读取的字节，不提前读取完整响应、不 drain、不改变业务返回的字节或错误、不发起额外上游调用。默认限制：

| 资源 | 上限 |
| --- | --- |
| 每个入站请求副本 | 4 MiB |
| 每次上游响应副本 | 8 MiB |
| 所有在途入站副本容量 | 32 MiB |
| 后台队列字节预算 | 32 MiB |
| 队列事件数 | 1024 |
| 每个正文块 | 32 KiB |

队列满、内存预算不足、磁盘错误时允许丢审计数据，业务继续。`incomplete=true`、`reason=body_limit/buffer_limit/queue_full` 或缺少终结事件均表示日志不完整。读取到 EOF 的完整事件会以 `outcome=eof` 结束；提前关闭、读取错误、取消或 handler/attempt 提前结束都会保留实际观察到的数据并标记不完整。提前关闭时底层返回错误会标记 `outcome=close_error`，原错误仍原样返回给业务；已经记录的 EOF/读取错误不会被后续 Close 覆盖。SSE 解析器在终止事件处主动结束时，也可能尚未读到 HTTP EOF。

写盘失败时，事件已消费但无法持久化，不保证文件中能补写丢失标记。通过配置区的「刷新状态」或 RootAuth `GET /api/option/audit/status` 查看 `dropped_events`、`write_errors`、`truncated_bodies`、`queue_bytes`、`buffered_bytes`、`last_error` 和 `config_error`。缺失、截断、写入错误必须结合这些计数判断，不能仅凭文件存在认定审计完整。

## 覆盖范围

覆盖完成用户令牌鉴权并进入 Gateway attempt 的 HTTP 请求和 SSE 响应，包括普通文本、原生协议、自定义 URL、HTTP 原始透传和通过 requester 执行的同步任务提交。Bedrock SDK 复用同一个被观察的 HTTP client；其 HTTP Body 可能是 AWS 二进制事件流，而非 SSE。

不包含 WebSocket 消息、MCP 管理/工具入口、鉴权失败请求、管理探测、OAuth/余额/模型发现等独立辅助调用、无用户令牌的媒体下载，以及脱离用户请求生命周期的后台任务轮询。只记录业务实际读取的响应；如果业务未读取错误 Body、客户端提前断开或流解析器提前结束，审计不会为补全内容额外读取。
