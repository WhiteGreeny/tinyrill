# 作为后台服务运行

先前台完成认证与实际业务验证。后台设置 `auth.interactive: never`；密码可用只读 secret 文件或已解锁的原生凭据服务，age-file 需要前台口令，不适合作为无交互服务密码源。

每客户一个普通用户进程、独立 YAML/状态目录/监听端口。无需系统网络管理员权限。程序自身仅对已有会话做有限恢复；外部管理器默认不要自动重启。退出 3/4 需人工认证或修改凭据，不能通过无限重启解决。

Linux 可使用用户级 systemd 服务（路径替换为实际绝对路径）：

```ini
[Unit]
Description=tinyrill customer-a
After=network-online.target

[Service]
Type=simple
ExecStart=/absolute/path/tinyrill run --config /absolute/path/customer-a.yaml
Restart=no
TimeoutStopSec=20
UMask=0077
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=default.target
```

保存为用户配置目录中的 `systemd/user/tinyrill-a.service`，执行 `systemctl --user daemon-reload`、`systemctl --user start tinyrill-a`。只读 secret 与状态路径必须对该用户可用。启用文件系统隔离时也需保留 Secret Service 的本地 D-Bus 访问；不要将整个凭据库挂入容器。

Windows 使用当前用户的任务计划或受支持的服务包装器，macOS 使用当前用户 LaunchAgent，传入的参数仅含 `run --config <绝对路径>`。具体进程管理器安装与策略由部署环境决定。前台挑战发生时停止后台实例，使用同一用户、配置和状态目录前台运行；独占锁会阻止两者同时使用。

stdout 是状态 JSON，stderr 是诊断；后台没有交互图像。判断业务可用应同时检查 `ready` 与业务探测。查看实际退出码；不要只以进程存在作为健康标志。停止进程后现有 TCP 连接会关闭，不承诺断线无损恢复。

容器运行见 [Podman 文档](docker.md)，升级和回退见 [验收表](acceptance.md)。