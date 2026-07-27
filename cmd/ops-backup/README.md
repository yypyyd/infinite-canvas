# ops-backup

数据库与七牛 Kodo 对象存储的离线备份、校验和恢复命令。支持 SQLite、MySQL 和 PostgreSQL，供停服维护或灾难恢复演练使用。

命令直接依赖项目 `config`、`repository` 和七牛 Go SDK；MySQL、PostgreSQL 流程还依赖运行镜像内的官方客户端。它只负责创建与恢复备份，不负责定时调度、加密、保留周期或异机复制。

## 使用方法

```bash
/app/ops-backup backup --output /app/data/backups
/app/ops-backup restore --input /app/data/backups/backup-... --confirm RESTORE
```

可用参数：

- `--skip-objects`：只处理数据库。
- `--overwrite-objects`：恢复时允许覆盖 Kodo 中内容不同的同名对象；默认遇到冲突立即退出。

备份目录包含 `manifest.json`、数据库快照和按对象键 SHA-256 命名的文件。完整停机流程见[备份与恢复](../../docs/content/docs/overview/backup-restore.mdx)。

## 目录结构

```text
ops-backup/
├── main.go       # 命令参数、流程编排和结构化日志
├── manifest.go   # 备份格式、路径边界和完整性校验
├── database.go   # 三种数据库的备份与恢复
└── objects.go    # Kodo 对象导出、预检和恢复
```

## 相关文档

- [设计文档](DESIGN.md)
