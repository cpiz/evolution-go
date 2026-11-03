# WhatsApp 握手版本修复

## 已确认

2026-11-03 生产日志同时出现新版 Web 版本和旧版 `2.3000.1042386815` 的 405 错误。原代码只设置伴随设备属性，未调用 `store.SetWAVersion`。

参考：https://github.com/evolution-foundation/evolution-go/issues/228 。社区成功案例不等于本环境验证结果。

## 实现与验证

- 进程启动时、账号连接及 HTTP 服务之前，优先采用完整的 `WHATSAPP_VERSION_MAJOR/MINOR/PATCH`；未指定时获取当前 Web 版本。
- 调用 `store.SetWAVersion` 同步实际握手版本和配对哈希。运行中不重复设置全局版本，下一次更新通过重启生效。
- 获取超时为 15 秒，非 200、无效版本或获取失败会明确中止启动。网络不可用时可使用经过验证的完整版本配置。
- 保留原 Go / whatsmeow 依赖和认证连接池修复。仅修复版本设置不能替代后续协议库升级。
- 回归：`go test ./pkg/whatsmeow/service ./cmd/evolution-go`；构建：仓库 Dockerfile。

## 发布与回滚

本项目的生产发布必须使用既有流程：代码提交并 push → 阿里云 ACR 自动构建 → 发布系统推送到 Dokploy。不得以本地镜像传输、手工修改 Dokploy 镜像或直接重建生产容器替代该流程。提交与 push 仍需用户明确要求。

发布前备份 users/auth 数据和运行数据卷，保存原镜像摘要。只更新 Evolution Go 镜像并重建该服务；不删除卷、不批量重建实例，不发布 Kowauto 应用。

验证启动日志的实际握手版本、405 是否消失、原会话是否恢复，以及二维码和消息/webhook 链路。401 失效会话可能需要操作者重新扫码；已有会话优先保留。

回滚可切回原镜像并重建服务，本补丁不新增数据库迁移。原镜像仍可能被 WhatsApp 拒绝，因此回滚不保证恢复连接。会话被远端注销不能靠镜像回退恢复。

## 本地验证状态

2026-11-03 已通过全项目 `go test ./...`、`go test -race ./pkg/whatsmeow/service`、相关包 `go vet`、格式和 diff 检查。现有 Dockerfile 的 linux/amd64 镜像构建成功；临时容器已实测握手版本初始化，随后按预期在不可达的测试数据库地址退出，未接入生产数据库。

本地检查证明版本设置逻辑与构建通过，不代表 WhatsApp 已接受连接。ACR 构建、发布系统投递、真实账号恢复、扫码及消息/webhook 仍需正常发布后验收。本次本地验证不修改 GitHub Actions、ACR 或发布系统配置。
