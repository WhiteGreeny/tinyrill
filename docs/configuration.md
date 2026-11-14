# YAML 与认证参考

完整入口以 `examples/` 和 `configs/profile.go` 的严格 schema 为准。所有相对路径以配置所在目录为基准；YAML 限制 64 KiB，拒绝未知/重复键、别名/锚点、自定义标签、类型转换和明文 password 字段。

| 字段 | 默认/用途 |
|---|---|
| `schema_version`, `id`, `protocol`, `server` | 必需；版本 1；客户 ID；easyconnect/atrust；网关主机[:端口] |
| `auth.method` | password；EC 支持 password/certificate，aTrust 支持 password/sms/cas/oauth2 |
| `auth.username`, `phone`, `domain` | 按认证方式指定；SMS 主认证使用 phone |
| `auth.remember_password` | false；只有显式 true 才自动读写密码后端 |
| `auth.secret_file`, `totp_secret_file` | 可选单行 secret 文件；保留密码首尾空格，去除末尾换行 |
| `auth.certificate_file`, `certificate_password_file` | EC PKCS#12 客户证书和独立口令文件 |
| `auth.interactive`, `timeout` | auto/never；单次交互默认 5m |
| `auth.captcha_mode`, `captcha_file` | auto/ansi/file；原图保存路径，file 模式必填 |
| `auth.callback_listen`, `callback_url` | 默认 127.0.0.1:0；容器固定监听端口及宿主回环 HTTP origin |
| `credentials.backend`, `ref`, `vault_file` | auto 或 age-file；非秘密引用默认客户 ID；age-file 必须指定 vault_file |
| `listen.socks5`, `http` | 127.0.0.1:1080/1081；空字符串禁用对应入口，至少保留一个 |
| `dns.mode`, `servers`, `ttl` | vpn；最多两个 IPv4 VPN 内解析器；缓存 TTL 默认 300s |
| `session.state_dir` | 默认配置目录下 state/id；一实例独占，不能复用 |
| `session.setup_timeout`, `dial_timeout` | 10m / 15s；认证初始化和业务拨号截止时间 |
| `session.reconnect_attempts` | 3，允许 0..10；进程生命周期内恢复预算，只有已有会话可自动恢复 |
| `tls.mode`, `ca_file`, `server_name`, `pins` | verify；可选私有 CA、身份名或可信 SPKI pin；insecure 必须明确配置 |
| `easyconnect.multi_line`, `allow_legacy_tls` | false；可选网关线路探测与显式旧数据 TLS 兼容 |
| `atrust.mac_addresses` | 可选 MAC 标识列表，不修改主机网卡 |
| `atrust.session_refresh`, `nodes_refresh` | 30m / 5m |
| `debug` | false；开启仍不输出认证正文、秘密或数据包内容 |

原生密码后端：Windows 当前用户 Credential Manager；macOS CGO 原生 Keychain；Linux 已解锁的 Secret Service，且仅连接本地 Unix D-Bus。服务不可用或锁定时无自动明文降级。age-file 使用 age 标准口令加密，需隐藏输入独立解锁口令，每客户单独 vault_file；不支持多个进程共享一个可写 vault。

认证成功但凭据保存失败时仍可运行，会明确提示未保存。`remember_password: false` 不删除已有条目，`credentials delete` 才删除；`session clear` 单独清除会话复用数据。状态目录和 age 文件由应用收紧为 Unix 0700/0600 或 Windows 当前用户专属 DACL；部署方注入的只读 secret 文件由部署方管理权限，程序只检查类型和大小。

普通图片验证码在终端展示并保存受保护原图；点选验证码使用一次性回环网页，支持清除、刷新及取消。file 模式通过原图和坐标 JSON（如 `[[12,34],[56,78]]`）交互。坐标按原图像素，最多 16 点。后台 `interactive: never` 或无终端遇到人工认证时返回 3，不反复发短信。CAS/OAuth2 链接由用户在浏览器打开，回填受支持流程的响应，终端不会自动执行网关程序。

stdout 输出 JSON 状态事件，stderr 为诊断和前台交互。状态包含客户、代次、状态、PID 和时间，保存在独立 status.json；认证链接/图像仅供交互，不应把交互流当作可公开日志。恢复会重建解析缓存和网络栈，旧 TCP 连接关闭，由应用自行重连。

连接就绪和每次恢复成功后，stderr 默认显示客户、代理监听地址与授权资源摘要（最多 20 条去重规则）。`tinyrill run --config customer.yaml --resources full` 显示完整列表，`--resources none` 关闭展示；该选项仅作用于本次运行，不改变 YAML 或路由规则。列表包含已解析的域名、IPv4 地址/范围、协议和端口；`permitted` 表示规则允许 TCP，UDP/ICMP 标为 `unsupported`。这是网关授权信息，没有主动探测可达性，也不包含业务系统名称。资源信息属于客户网络信息，保存 stderr 日志时按内部资料管理。

排查资源名称字段时，使用 `tinyrill run --config customer.yaml --resource-debug`。这与 YAML 的 `debug` 独立，不需要开启普通 debug。取得资源后将字段结构写入该客户 `session.state_dir` 下的 `resource-fields.json`，终端提示实际路径。文件只含采集时间、协议、字段名/层级和类型，不保存字段原值，因此没有资源名称、域名/IP、资源 ID、密码或令牌的原值。EC 仅检查 `Resource/Rcs`，aTrust 仅检查 `data.appList.data.appInfo`；不是认证响应或完整资源响应转储。

诊断默认关闭；数组/同名 XML 节点最多取 8 项，遍历最多 512 节点、8 层、128 种字段，达到边界时标记 `truncated`。异常或动态字段名会被过滤并标记 `filtered_names`。这是字段结构样本，不能据此断言其他资源或接口没有名称。文件使用私有权限并在下次采集时原子替换；不开启时不会产生新文件，也不会自动删除旧报告。分享前检查采集时间和字段名，只提供这个文件即可，不需要提交配置、状态会话或完整日志。

退出码：0 成功/正常停止；2 配置/监听冲突；3 需要人工认证；4 认证拒绝；5 连接失败；6 状态/凭据错误。初次登录失败不自动重试密码；已有会话恢复耗尽后退出。外部进程管理器不要无限重启。
