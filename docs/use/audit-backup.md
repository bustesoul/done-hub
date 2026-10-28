# 审计归档与宿主机备份

归档模式中，Done Hub 负责写入和封口，宿主机负责上传、校验和清理。Done Hub 不读取网盘地址或凭据、不发送备份请求、不删除待备份日志。普通模式默认保持原有的 100 MiB / 7 天 / 10 份保留策略。

## 配置与文件生命周期

先准备宿主机备份任务，再在「设置 → 运营设置 → 输出审计」打开「归档模式」并保存。也可使用现有 RootAuth `PUT /api/option/`：

```json
{
  "key": "AuditLogConfig",
  "value": "{\"channel_ids\":[35],\"token_ids\":[],\"archive_enabled\":true}"
}
```

`archive_enabled` 默认 `false`，省略时仍为普通模式。它不改变渠道 / 用户令牌筛选规则；两组 ID 都为空时不记录新请求。「关闭审计」清空筛选，保留选择的文件管理模式。

归档目录默认为：

```text
容器：/data/logs/audit/archive/
宿主：/home/donehub/data/logs/audit/archive/
```

单个文件达到 100 MiB 前轮转，或自第一条写入经过 5 分钟后封口；后台每秒检查一次，磁盘写入阻塞时可能延迟。无数据时不创建空归档。名字包含 UTC 时间与随机后缀，防止快速轮转或重启覆盖：

```text
audit-20261028T010000.000000000Z-123456789.open  # 活动文件，备份任务忽略
audit-20261028T010000.000000000Z-123456789.log   # 已封口，不再修改
```

封口时同步文件、关闭句柄，再在同目录重命名为 `.log`。正常退出会在有界审计关闭流程中封口；异常退出留下的 `.open` 会在下次启动后的后台维护中恢复，即使此时已关闭审计或切回普通模式。恢复保留实际字节，包括崩溃可能留下的不完整末行，不伪造完整事件。

每个实例必须使用独立的本地日志目录。目录锁防止第二个进程误封口仍在写入的文件；不支持 `flock` 的平台开启归档模式会报告写入错误，普通模式仍可使用。

配置发布不等待磁盘，后续后台写入使用最新文件模式；切换时已有事件可能分布在普通文件和归档文件中，按 `request_id` 关联即可。关闭归档后会封口当前归档文件，历史 `.log` 仍保留。**此前普通目录中的 `audit.log` 和轮转文件不会自动迁移到归档目录**，应在切换前按原方式独立备份；不要移动仍在写入的普通文件。

现有 lumberjack 封装继续用于普通模式。它的 `Close` 不发布封口文件、时间戳轮转可能复用原名，无法直接表达“唯一 `.open` → 不可变 `.log`”和重启恢复，因此归档部分使用最小的文件 writer；继续复用原 Recorder 的有界队列、错误计数和关闭超时。

## 宿主机备份脚本

脚本为 [`bin/audit-backup.py`](../../bin/audit-backup.py)，仅依赖 **Python 3.9+ 和 rclone**。宿主机需自行安装这两个工具，Done Hub 容器内不需要安装。示例 service/timer 不会由程序自动安装或启用。

每次任务持有独占锁，只处理 `audit-*.log`，逐文件执行：

1. 将原文件压缩到目录内权限为 `0600` 的临时 gzip，原件保持不动。一次只保留一个临时压缩文件，重试时 gzip 内容相同。
2. `rclone copyto --ignore-existing` 上传到配置的目标，文件名为原名加 `.gz`。已有同名对象一律跳过上传，随后必须回读验证，不覆盖历史对象。
3. `rclone cat` 回读远端压缩文件，流式计算 SHA-256，与本地压缩文件比较，不依赖网盘是否提供 hash，也适用于 crypt remote。
4. 确认源文件仍为同一 inode、大小和修改状态后，才删除该本地 `.log`。`--keep-source` 可完成验证而保留原件。

任何一步失败都保留原件，继续尝试其他文件并以非零状态退出。上传和回读均由父进程强制限制执行时间（默认每个命令 330 秒，可用 `--command-timeout` 调整）。进程被终止后原件仍在；下一次任务清理自身残留的临时 gzip 并重试。上传后、删除前崩溃也能通过相同文件名和内容再次验证。源目录中的 symlink 不会被当作日志上传。

脚本输出 JSON 状态到 stdout，包含成功文件、压缩内容 SHA-256、失败原因、待备份文件数和字节数，不打印日志正文、rclone 凭据或工具的原始输出。上传或校验失败时，先检查 `rclone` 配置与网络；远端同名对象损坏需要人工检查，脚本不会覆盖它或删除本地原件。

配置示例见 [`bin/audit-backup.env.example`](../../bin/audit-backup.env.example)：

```ini
AUDIT_ARCHIVE_DIR=/home/donehub/data/logs/audit/archive
AUDIT_BACKUP_DEST=mydrive:donehub-audit/production
RCLONE_CONFIG=/etc/donehub/rclone.conf
```

`mydrive` 是通过 `rclone config` 配置的 named remote；宿主机凭据文件应设为 `0600`，不要提交进仓库。每个 Done Hub 实例应使用不同的远端子目录。

首次手工验证（环境变量也可通过运维的配置管理注入）：

```bash
sudo env AUDIT_BACKUP_DEST='mydrive:donehub-audit/production' \
  RCLONE_CONFIG=/etc/donehub/rclone.conf \
  python3 /usr/local/lib/donehub/audit-backup.py --keep-source
```

已有真实挂载目录时，将 `AUDIT_BACKUP_DEST` 设为绝对路径，并设置 `AUDIT_BACKUP_MOUNT` 为实际挂载点。脚本在上传前、删除前确认挂载仍存在，挂载失效时保留原件。**FUSE 网盘的写入/回读可能命中本地缓存，不能证明云端已持久化**，这类网盘应优先使用直接 rclone remote。

不带挂载的本地路径需要显式 `--allow-local`，主要用于开发测试，不会默认将网盘掉线后的普通目录当作可用备份盘。

## systemd 调度示例

安装位置约定：

| 仓库文件 | 宿主机位置 |
| --- | --- |
| `bin/audit-backup.py` | `/usr/local/lib/donehub/audit-backup.py` |
| `bin/audit-backup.env.example` 的实际配置 | `/etc/donehub/audit-backup.env` |
| `bin/systemd/donehub-audit-backup.service` | `/etc/systemd/system/donehub-audit-backup.service` |
| `bin/systemd/donehub-audit-backup.timer` | `/etc/systemd/system/donehub-audit-backup.timer` |

service 限制 CPU 25%、内存 256 MiB，20 分钟超时；timer 每分钟检查，停机错过的调度在恢复时补跑。运行中的同名 service 不会被 timer 重复启动，脚本锁也防止手工任务与定时任务重叠。

若 rclone 不在 systemd 的 PATH 中，使用 service drop-in 显式指定路径，例如先用空的 `ExecStart=` 清除原命令，再设置 `ExecStart=/usr/bin/python3 /usr/local/lib/donehub/audit-backup.py --rclone /home/rclone/bin/rclone`。不要依赖交互终端的 PATH。

运维确认目标、权限和首次回读验证后，再执行：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now donehub-audit-backup.timer
sudo journalctl -u donehub-audit-backup.service
```

对 fstab 管理的挂载目标，可在 service 的部署覆盖配置中增加 `RequiresMountsFor=/实际挂载路径`。停止 timer 不影响 Done Hub，但待备份文件将积压。**部署脚本和单元文件不会触发 Done Hub 重启。**

## 容量与保证边界

归档模式不会自动按 7 天或 10 份删除。网盘正常、上传速度覆盖生成速度时，本地只保留活动文件和待上传文件；压缩时额外占用最多一个文件的压缩大小。上传加回读约消耗两次压缩文件大小的网络流量，应根据日志生成速度配置备份资源。

必须监控 `pending_bytes`、任务失败状态和宿主机磁盘剩余空间，建议积压 1 GiB 提前告警。**当前脚本只备份和清理已确认的原件，不调用 Done Hub API，也不自动关闭审计。** 网盘长期故障时，应扩容或由运维暂停新增审计；需要硬隔离时，把归档目录放在独立、受限的本地文件系统，避免挤占业务数据库所在盘。未备份文件不能通过自动删除来满足容量上限。

这里保证的是“尚未验证备份成功的历史日志不会被主动清理”。它不改变审计原有的旁路策略：队列满、磁盘故障、超时退出或响应提前结束仍可能使采集不完整；宿主机损坏前尚未上传的文件也存在风险。文件同步和回读验证不是跨本地磁盘与网盘的事务，也不替代独立备份目标和恢复验证。

## 本地回归

```bash
go test -race ./internal/audit ./model ./middleware ./common/requester ./providers/midjourney
RCLONE_BIN=/path/to/rclone python3 -m unittest discover -s bin -p 'test_audit_backup.py' -v
```

脚本回归通过真实 rclone local backend 完成，无需网盘账号；缺少 rclone 时会明确跳过，应安装或设置 `RCLONE_BIN` 后执行完整回归。

参考：[rclone copyto](https://rclone.org/commands/rclone_copyto/)、[rclone cat](https://rclone.org/commands/rclone_cat/)、[rclone mount 缓存](https://rclone.org/commands/rclone_mount/)。
