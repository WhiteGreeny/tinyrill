# 实施记录

实施基线：`8576e7a16076c4175d6dbe276573a353cf7260be`。范围以 `ec-atrust-fork-plan.md` 为准。

## 基线验证

2026-11-13，Windows amd64，Go 1.27.2：依赖下载后运行 `go test -count=1 -timeout=120s ./...`。除 `internal/keylog/TestOpen` 与 `underlay/TestPCAPCaptureRecordsTCPInBothDirections` 外均通过。两项失败均为 Windows 文件 mode 返回 0666、测试要求 Unix 0600；安全文件实现和验收需改用 Windows ACL，不以跳过权限测试解决。

## 交付进度

- 已固定实施方案，首版容器为同一 Linux 二进制的轻量封装；宿主凭据联动延后。
- 已完成：业务拨号与 DNS 禁止直连回退，移除业务直连实现、HTTP 环境代理和 SOCKS5 UDP/BIND；`go test -count=1 -timeout=120s ./dial ./resolve ./service` 通过。
- 待完成：严格 YAML/CLI、功能裁剪、生命周期/状态隔离、TLS/日志/输入保护、认证/凭据、部署/CI、综合验收。

## 外部验收

以下尚未验证，不能作为已支持能力发布：实际 EC 7.6.3、EC 7.6.7、aTrust 客户网关与业务；Linux/macOS 系统凭据库；rootless Podman；三个客户并行与 8 小时稳定性。继续完成可控测试，客户凭据不进入仓库或日志。
