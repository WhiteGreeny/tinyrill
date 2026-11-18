# 配置指南与逐项参数参考

适用于 YAML `schema_version: 1`；`dns.hosts` 需要 RC5 或更新版本。普通用户先按下面的步骤修改示例，有特殊需求时再查逐项参考。本文覆盖全部 42 个配置字段，默认值和限制以当前实现核对。

## 第一次使用：哪些需要改

认证异常的采集使用命令行 `--auth-debug`，无需新增 YAML 字段或开启 `debug: true`；见 [认证阶段诊断](auth-diagnostics.md)。

1. 原生 EC 从 [customer-a-ec.yaml](../examples/customer-a-ec.yaml) 复制，aTrust 从 [customer-c-atrust.yaml](../examples/customer-c-atrust.yaml) 复制；容器使用 [customer-container.yaml](../examples/customer-container.yaml)，配合 [Podman 说明](docker.md)。
2. **必须修改**示例中的 `server` 和密码登录的 `auth.username`；给客户选择固定、独立的 `id`。`schema_version` 保留 `1`，按客户端类型确认 `protocol`。
3. **多客户或端口冲突时调整** `listen.socks5`、`listen.http` 和显式填写的 `session.state_dir`。修改 `id` 不会自动改写示例中的端口与路径。
4. **按需开启**记住密码、静态域名映射、私有 CA 或特殊认证；没有对应需求就保留默认值。EC 旧通道还需按下表确认兼容开关。
5. 先做配置检查，再前台登录。Windows 使用 `./tinyrill.exe`，Linux 使用 `./tinyrill`：

```powershell
./tinyrill.exe config-check --config examples/customer-a-ec.yaml
./tinyrill.exe run --config examples/customer-a-ec.yaml
```

```sh
./tinyrill config-check --config examples/customer-a-ec.yaml
./tinyrill run --config examples/customer-a-ec.yaml
```

`config-check` 检查 YAML、字段、必填条件和 TLS CA/pin 配置，不连接网关、不占用代理端口，也不检查所有 secret/客户端证书文件、密码或业务可达性。检查通过后仍需登录并访问实际业务。修改 YAML 后重启对应客户实例生效。

| 你的需求 | 应配置什么 | 没有这个需求时 |
|---|---|---|
| 用户名和密码登录 | `auth.method: password`、`auth.username`；密码在运行时输入 | 示例已选此方式；不要添加 `password` 字段 |
| aTrust 有认证域或多种登录方式 | 先运行 `auth-info --config ...`，按结果填写 `auth.domain` 和 `auth.method` | 只有网关对应方式的域确实为空时才保留 `domain: ""` |
| 记住密码 | `auth.remember_password: true` | 默认 `false`，不会自动读写已保存密码 |
| 多个客户同时使用 | 每客户独立 ID、监听端口、状态目录 | 单客户可使用示例端口；同时启动前检查冲突 |
| 固定某个业务域名的地址 | `dns.hosts` | 省略，由网关映射/VPN DNS 处理 |
| 网关没有合适的 DNS，需要指定内部解析器 | `dns.servers`，填写经该 VPN 可达的 IPv4 地址 | 省略或 `[]` 使用网关下发的 DNS，**不是禁用 DNS** |
| 网关使用私有证书 | `tls.mode: verify` 和管理员提供的 `tls.ca_file` | 默认校验证书；不要用 `insecure` 作为通用修复 |
| EC 数据通道提示需要 TLS 1.1/RC4 | 确认该客户接受旧协议后设 `easyconnect.allow_legacy_tls: true` | 默认 `false`；此开关不关闭服务器身份校验 |
| 后台/容器注入密码 | `auth.secret_file`；无人值守时同时设 `auth.interactive: never` | 前台登录无需密码文件；有人工挑战的流程仍需人工完成 |
| 查看更多连接诊断 | 临时设 `debug: true` | 默认 `false`；普通失败日志和资源列表已经可用 |

单客户原生密码登录的最小配置如下。它使用默认代理端口 `127.0.0.1:1080/1081`、默认状态目录 `state/customer-a`，只表示参数齐全；网关要求的认证域、证书或 EC 兼容开关仍需按实际配置。

```yaml
schema_version: 1
id: customer-a
protocol: easyconnect
server: "vpn.customer-a.example:443"
auth:
  username: "replace-me"
```

## 如何填写 YAML

- **省略字段**使用下表默认值；显式写 `""`、`false`、`[]`、`{}` 可能具有不同含义。例如监听地址省略会启用默认端口，写 `""` 才关闭该入口。不要写空的 `dns:`、`hosts:` 或 `null`，直接省略不用的项。
- 所有相对文件/目录路径都相对于 **YAML 所在目录**，不相对于执行命令的目录。默认状态目录也在 YAML 目录下；示例显式使用 `../state/...`，因此复制 YAML 到其他目录会改变路径位置。
- Windows 路径可以用 `C:/vpn/ca.pem` 或单引号 `'C:\vpn\ca.pem'`，避免双引号中的反斜杠转义。用户名、手机号和域建议加引号，避免纯数字被识别为数字。
- 布尔值使用 `true`/`false`，整数不加引号；时间使用带单位的字符串，如 `15s`、`5m`、`1h`。时间字段统一允许 `1s` 到 `24h`，`0` 不表示禁用。
- 同一节只写一次。下面的场景片段应合并进已有 `auth`、`dns` 等节，不要在文件末尾重复添加同名节。
- 文件必须为 `.yaml`/`.yml`，一个文档、最多 64 KiB；拒绝未知/重复键、类型转换、YAML 锚点/别名、显式标签和明文 `password` 字段。不支持环境变量覆盖或 `${VAR}` 展开。

## 逐项参考

表中“无”表示省略时为空/不配置，“默认路径”按上述相对路径规则解释。除标注为 EC/aTrust 专用的字段外，两种协议共用。专用字段只用于相应协议，不要将另一协议的设置复制进来。

### 客户身份与网关

| 参数 | 类型、默认值与允许值 | 何时填写、作用与限制 |
|---|---|---|
| `schema_version` | 整数；无默认；只允许 `1` | 必填，保留示例值，不是客户端或网关版本号 |
| `id` | 字符串；无默认；1–64 个 ASCII 字母、数字、点、下划线或连字符，首字符为字母/数字 | 必填，客户的固定标识，用于状态及凭据隔离；同一客户日常使用保持稳定 |
| `protocol` | 字符串；无默认；`easyconnect` / `atrust` | 必填，选择网关产品；EC 7.6.3/7.6.7 都填 `easyconnect` |
| `server` | 字符串；无默认；主机名或 IP，可带 `:端口`，省略端口为 `443` | 必填，例如 `vpn.example:443`；不带 `https://`、路径、账号或查询参数，端口 1–65535 |

### 登录方式与交互

| 参数 | 类型、默认值与允许值 | 何时填写、作用与限制 |
|---|---|---|
| `auth.method` | 字符串；默认 `password`；EC：`password` / `certificate`；aTrust：`password` / `sms` / `cas` / `oauth2` | 选择主认证方式；登录后的短信、TOTP、图片/点选验证码由网关挑战触发，无需把主认证改成验证码类型 |
| `auth.username` | 字符串；默认无 | `password` 必填；填写实际账户，不填密码；其他方式按账户标识需要填写 |
| `auth.phone` | 字符串；默认无 | 仅 aTrust `sms` 主认证必填，例如 `"13800000000"`；不要写成 YAML 数字 |
| `auth.domain` | 字符串；默认 `""` | aTrust 登录域，必须与网关的认证方式/域组合对应；用 `auth-info` 查到的 `domain` 原样填写，不是业务 DNS 后缀；EC 不使用 |
| `auth.remember_password` | 布尔；默认 `false` | 只适用于 `password`；`true` 才自动读取凭据并在成功认证后保存；`false` 不会删除旧密码 |
| `auth.secret_file` | 路径字符串；默认无 | 用密码文件替代交互输入，主要用于后台/容器；读取优先于记住的密码，读取失败不会改用其他来源；若同时开启记住密码，认证成功仍会保存该密码 |
| `auth.totp_secret_file` | 路径字符串；默认无 | 需要程序生成 TOTP 时填写种子文件，内容是种子而非当前验证码；省略时遇到 TOTP 按提示输入 |
| `auth.certificate_file` | 路径字符串；默认无 | EC `certificate` 主认证必填，PKCS#12（`.p12`/`.pfx`）客户端证书，最多 4 MiB；与网关 CA 文件用途不同 |
| `auth.certificate_password_file` | 路径字符串；默认无 | EC 客户端证书的口令文件；省略会隐藏询问口令，无口令证书交互时留空；文件本身不能是空行 |
| `auth.interactive` | 字符串；默认 `auto`；`auto` / `never` | `auto` 在有终端时交互；`never` 禁止人工输入，遇到需要人工的认证退出 3；没有终端也无法完成交互 |
| `auth.timeout` | 时长；默认 `5m`；`1s`–`24h` | 单次密码/认证交互的等待时间，也受当前初始化截止时间约束；通常保留默认 |
| `auth.captcha_mode` | 字符串；默认 `auto`；`auto` / `ansi` / `file` | `auto` 自动选择普通图片终端显示方式；`ansi` 使用 ANSI；这两种模式的点选验证码使用临时本机网页。`file` 使用原图和文字/坐标输入，必须显式设置 `captcha_file` |
| `auth.captcha_file` | 路径字符串；默认在状态目录下生成 `captcha.png` | 改变验证码原图保存位置；`file` 模式必须填写，即使其他模式可自动选默认路径；设置路径本身不会禁用点选网页 |
| `auth.callback_listen` | 地址字符串；默认 `127.0.0.1:0` | 点选验证码网页监听地址，必须为 IP:端口，`0` 为临时端口；原生一般不改。非回环监听必须指定固定端口和 `callback_url` |
| `auth.callback_url` | URL 字符串；默认按网页实际监听地址生成 | 容器端口映射时填用户浏览器可访问的回环 HTTP 地址，如 `http://127.0.0.1:21999`；只接受回环 IP 和 1–65535 端口，不带账号、查询、片段或业务路径；不是 CAS/OAuth2 的认证回调参数 |

密码、TOTP 种子和证书口令文件均为一个非空文本行，内容最多 4096 字节；保留首尾空格，仅移除末尾换行，不能含内嵌换行或 NUL。按需配置引用，文件内容不要写进 YAML 或命令行。

### 密码保存后端

| 参数 | 类型、默认值与允许值 | 何时填写、作用与限制 |
|---|---|---|
| `credentials.backend` | 字符串；默认 `auto`；`auto` / `age-file` | `auto` 使用当前系统凭据库；没有可用系统凭据库且需要加密文件时选 `age-file`，同时填写 `vault_file`，运行时需独立解锁口令。选择后端本身不启用记住密码 |
| `credentials.ref` | 字符串；省略/空字符串时使用 `id`，最多 128 字节 | 仅需要自定义密码条目标识时填写；它不是密码，也不覆盖客户、网关、账户的身份绑定；通常省略 |
| `credentials.vault_file` | 路径字符串；默认无 | 仅 `age-file` 使用且必填，例如 `secrets/customer-a.age`；每客户单独文件，不要让多个进程共享可写 vault；`auto` 不读取该文件 |

### 代理监听

| 参数 | 类型、默认值与允许值 | 何时填写、作用与限制 |
|---|---|---|
| `listen.socks5` | 地址字符串；默认 `127.0.0.1:1080`；`""` 禁用 | SOCKS5 TCP 入口；客户端用 `socks5h` 传递域名。不支持 UDP/BIND，多客户需不同端口 |
| `listen.http` | 地址字符串；默认 `127.0.0.1:1081`；`""` 禁用 | HTTP/HTTPS CONNECT 代理入口，多客户需不同端口 |

监听地址必须是明确 IP:端口，端口 1–65535，不能写 `localhost` 或 `0`；HTTP 与 SOCKS5 使用不同地址，至少保留一个入口。原生通常用 `127.0.0.1`；容器内监听 `0.0.0.0`，主机只发布回环端口。这两个入口没有代理用户名/密码认证，不要随意对局域网开放。

### 客户内部 DNS

| 参数 | 类型、默认值与允许值 | 何时填写、作用与限制 |
|---|---|---|
| `dns.mode` | 字符串；默认 `vpn`；只允许 `vpn` | 通常省略或保持默认；没有 `system`、`direct`、`auto` 等模式 |
| `dns.servers` | 字符串列表；默认 `[]`；最多两个 IPv4 地址 | 省略/空列表时采用网关 DNS；显式列表替代网关下发的 DNS，按主/备用使用，查询仍经 VPN；填 IP，不填端口、域名或 DoH URL |
| `dns.ttl` | 整数秒；默认 `300`；1–86400 | 动态解析缓存时间；不是时长字符串，不能写 `5m`；静态 hosts 不受此 TTL 影响 |
| `dns.hosts` | 字符串到字符串的映射；默认无；最多 256 条 | RC5 起可固定业务域名到 IPv4；精确名称或 `+.` 后缀，精确优先、最长后缀次之；不增加资源权限，详见后文 |

### 会话与超时

| 参数 | 类型、默认值与允许值 | 何时填写、作用与限制 |
|---|---|---|
| `session.state_dir` | 路径字符串；省略/空字符串时为 YAML 目录下 `state/<id>` | 保存会话、状态及默认验证码/诊断文件；每实例独占。示例显式写 `../state/...`，改 ID 时须同步修改显式路径 |
| `session.setup_timeout` | 时长；默认 `10m`；`1s`–`24h` | 每次会话认证/初始化的截止时间，也用于 `auth-info` 查询；认证较慢时调整，不是总在线时长 |
| `session.dial_timeout` | 时长；默认 `15s`；`1s`–`24h` | 业务连接建立及网络栈启动的截止时间，不是已连接业务的空闲断开时间 |
| `session.reconnect_attempts` | 整数；默认 `3`；0–10 | 进程生命周期内、曾就绪后的会话恢复预算；`0` 关闭自动恢复，不会自动重试首次失败的密码登录，成功恢复后也不重置预算 |

### 网关 TLS 身份校验

| 参数 | 类型、默认值与允许值 | 何时填写、作用与限制 |
|---|---|---|
| `tls.mode` | 字符串；默认 `verify`；`verify` / `pin` / `insecure` | `verify` 校验证书链和主机名；`pin` 用独立取得的公钥指纹替代常规链/主机名校验；`insecure` 跳过身份校验，仅用于明确接受的客户例外 |
| `tls.ca_file` | 路径字符串；默认无 | `verify` 模式下，私有 CA 的 PEM 文件，最多 1 MiB；提供后使用文件中的 CA 集合替代系统根集合，文件需包含该配置涉及的受信任 CA；`pin`/`insecure` 不靠它校验身份 |
| `tls.server_name` | 字符串；默认按实际连接的网关/节点主机名确定 | 仅管理员指定固定的证书身份名/SNI 时填写，例如按 IP 连接而证书签给特定域名；此覆盖会用于该客户的连接，不是业务资源域名 |
| `tls.pins` | 字符串列表；默认 `[]`；最多 8 条 | 仅 `pin` 模式使用，且该模式至少一条；格式 `sha256/` 加 Base64 编码的 SHA-256 SPKI 公钥摘要。指纹从可信渠道取得，其他模式不能填写非空列表 |

### EC / aTrust 专用参数与调试

| 参数 | 类型、默认值与允许值 | 何时填写、作用与限制 |
|---|---|---|
| `easyconnect.multi_line` | 布尔；默认 `false`；仅 EC | 明确需要探测网关提供的多条线路时开启；选中其他线路可能重新建立一次会话，通常保留默认 |
| `easyconnect.allow_legacy_tls` | 布尔；默认 `false`；仅 EC | 继承的 EC TCPP/L3 数据通道需要 TLS 1.1/RC4 时必须显式开启；它允许旧协议，但不等于 `tls.mode: insecure` |
| `atrust.mac_addresses` | 字符串列表；默认无；每项为合法 MAC 地址；仅 aTrust | 通常省略，使用会话内保存的标识或首次生成的标识；只有客户要求固定标识时填写。已有会话的 MAC 优先，修改配置不会直接覆盖旧会话标识；不修改主机网卡 |
| `atrust.session_refresh` | 时长；默认 `30m`；`1s`–`24h`；仅 aTrust | 会话刷新间隔，通常保持默认，`0` 不受支持 |
| `atrust.nodes_refresh` | 时长；默认 `5m`；`1s`–`24h`；仅 aTrust | 最佳节点刷新间隔，通常保持默认，`0` 不受支持 |
| `debug` | 布尔；默认 `false` | 排障时临时开启目标、DNS 来源和拨号详情；普通失败和资源列表不需要此开关，资源原值导出使用另一个命令行开关 |

## 特殊认证怎么选择

aTrust 先执行 `tinyrill auth-info --config customer.yaml` 查询；该命令不登录。输出的 `auth_type` 与 YAML 的 `auth.method` 对应如下，`domain` 应使用同一项的值，不能把显示名称 `name` 当成域。

| 协议/查询结果 | `auth.method` | 需要补充的配置 |
|---|---|---|
| EC 用户名密码 | `password` | `auth.username`，其他挑战按运行提示完成 |
| EC 客户端证书 | `certificate` | `auth.certificate_file`，口令可交互输入或通过 `certificate_password_file` 提供 |
| aTrust `auth/psw` | `password` | `auth.username`、对应 `auth.domain` |
| aTrust `auth/smsCheckCode` | `sms` | `auth.phone`、对应 `auth.domain`；短信验证码运行时输入 |
| aTrust `auth/cas` | `cas` | 对应 `auth.domain`，按提示打开认证链接并回填响应 |
| aTrust `auth/httpsOauth2` | `oauth2` | 对应 `auth.domain`，按提示打开认证链接并回填响应 |

非密码主认证保持 `remember_password: false`。EC 的 `auth-info` 仅列出本实现支持的方式，不发现网关认证能力；未列出的 aTrust 定制方式不能通过猜测 `method` 值启用。后台配置见 [服务运行](service.md)，容器回调和 secret 注入见 [Podman 说明](docker.md)。

## DNS 与连接诊断

`dns.mode` 目前只接受 `vpn`，省略时也使用该值。`dns.servers` 中的查询通过客户 VPN 发送；不启用系统或公共 DNS 回退。

从 RC5 起，可以在客户 YAML 中显式补充静态映射，例如：

```yaml
dns:
  mode: vpn
  hosts:
    "portal.apps.internal": "10.0.0.20"
    "+.apps.internal": "10.0.0.10"
```

精确名称优先于后缀；多个后缀命中时选择最长后缀。上例的 `portal.apps.internal` 使用 `10.0.0.20`，`apps.internal`、`dev.apps.internal`、`a.b.apps.internal` 使用 `10.0.0.10`，`notapps.internal` 不匹配。比较忽略大小写及一个结尾的点；规范化后重复的规则拒绝加载。只接受 ASCII 主机名和 IPv4 值，不接受 `*.`、正则、URL、端口、IPv6 或另一个域名作为值。

显式映射优先于本实例的缓存、网关资源映射和 VPN DNS，不发起 DNS 查询。未命中时继续原有解析流程。配置只影响所属客户，修改后重启该实例生效；不修改主机 hosts、系统 DNS 或其他代理配置。HTTP Host 和 TLS 使用的目标域名仍由应用保留。映射不新增资源权限，目标仍需满足网关下发的域名或 IP/协议/端口授权。

启动时 stderr 显示 `DNS ready`，列出静态映射数、网关映射数和可用 VPN DNS 服务器数；这表示解析能力已配置，不表示已探测业务可达性。普通日志会显示代理失败阶段：`stage=dns` 为解析失败，`stage=authorization` 为资源授权拒绝，`stage=dial` 为建立 VPN 业务连接失败，`stage=session` 为会话不可用，`stage=http` 为 HTTP 转发失败。同一实例、阶段、目标和错误在 30 秒内限频，下一次输出的 `suppressed` 记录期间省略次数；仅静默期间不会补发汇总。

`debug: true` 额外输出代理收到的目标、DNS 命中的来源/地址、VPN DNS 服务器和拨号成功信息，并关闭上述失败限频。正常取消不作为普通错误。日志不包含认证正文或数据包，但目标域名/IP 属于客户网络信息，分享前自行筛选。`stdout` 继续仅输出 JSON 状态事件。与 mihomo 联用的排障步骤见 [多客户与 mihomo](mihomo.md)。

## 认证、状态与资源

原生密码后端：Windows 当前用户 Credential Manager；macOS CGO 原生 Keychain；Linux 已解锁的 Secret Service，且仅连接本地 Unix D-Bus。服务不可用或锁定时无自动明文降级。age-file 使用 age 标准口令加密，需隐藏输入独立解锁口令，每客户单独 vault_file；不支持多个进程共享一个可写 vault。

认证成功但凭据保存失败时仍可运行，会明确提示未保存。`remember_password: false` 不删除已有条目，`credentials delete` 才删除；`session clear` 单独清除会话复用数据。状态目录和 age 文件由应用收紧为 Unix 0700/0600 或 Windows 当前用户专属 DACL；部署方注入的只读 secret 文件由部署方管理权限，程序只检查类型和大小。

普通图片验证码在终端展示并保存受保护原图；点选验证码使用一次性回环网页，支持清除、刷新及取消。file 模式通过原图和坐标 JSON（如 `[[12,34],[56,78]]`）交互。坐标按原图像素，最多 16 点。后台 `interactive: never` 或无终端遇到人工认证时返回 3，不反复发短信。CAS/OAuth2 链接由用户在浏览器打开，回填受支持流程的响应，终端不会自动执行网关程序。

stdout 输出 JSON 状态事件，stderr 为诊断和前台交互。状态包含客户、代次、状态、PID 和时间，保存在独立 status.json；认证链接/图像仅供交互，不应把交互流当作可公开日志。恢复会重建解析缓存和网络栈，旧 TCP 连接关闭，由应用自行重连。

连接就绪和每次恢复成功后，stderr 默认显示客户、代理监听地址与授权资源摘要（最多 20 条去重条目）。`tinyrill run --config customer.yaml --resources full` 显示完整列表，`--resources none` 关闭展示；该选项仅作用于本次运行，不改变 YAML 或路由规则。列表包含已解析的域名、IPv4 地址/范围、协议、端口和资源说明；`permitted` 表示规则允许 TCP，UDP/ICMP 标为 `unsupported`。这是网关授权信息，没有主动探测可达性。资源信息属于客户网络信息，保存 stderr 日志时按内部资料管理。

从 RC4 起，`RESOURCE` 列按下面的标签分别显示有值的字段；续行位于同一列。aTrust 分组和应用的名称、描述独立保留，EC 名称与备注也独立保留：

| 协议 | 网关字段 | 展示标签 |
|---|---|---|
| aTrust | `data.appList.data.appInfo[].name` | `Group name` |
| aTrust | `data.appList.data.appInfo[].description` | `Group description` |
| aTrust | `data.appList.data.appInfo[].apps[].name` | `Name` |
| aTrust | `data.appList.data.appInfo[].apps[].description` | `Description` |
| EC | `Resource/Rcs/Rc/@name` | `Name` |
| EC | `Resource/Rcs/Rc/@note` | `Note` |

名称为空时，非空描述或备注会直接显示；名称和描述同时有值时两者都显示，即使文字相同也不互相覆盖。所有说明字段都为空时显示 `-`，地址仍保留。同一资源展开出的域名、单个 IP、IP 范围、CIDR 或网关 DNS 对应的 IP 继承其说明；相同目标/协议/端口但任一说明字段不同的条目分别展示，去重仅影响显示。

上述文本不参与授权匹配或隧道选择。资源模型保留完整文本，展示时修剪首尾空白、过滤终端控制字符，每个字段最多显示前 256 个字符，超长追加 `...`。EC XML 实体正常解码，不额外猜测 URL 编码。正常展示无需开启 debug，原值诊断仍可用于查看完整文本。

排查资源名称字段时，使用 `tinyrill run --config customer.yaml --resource-debug`。这与 YAML 的 `debug` 独立，不需要开启普通 debug。取得资源后将字段结构写入该客户 `session.state_dir` 下的 `resource-fields.json`，终端提示实际路径。文件只含采集时间、协议、字段名/层级和类型，不保存字段原值，因此没有资源名称、域名/IP、资源 ID、密码或令牌的原值。EC 仅检查 `Resource/Rcs`，aTrust 仅检查 `data.appList.data.appInfo`；不是认证响应或完整资源响应转储。

需自行查看实际值时，使用 `0.1.0-rc.3-diagnostic.1` 或后续构建的 `--resource-debug-values`。该独立开关还会在同一状态目录写入 `resource-values.json`（aTrust）或 `resource-values.xml`（EC），保留上述资源分支的全部字段、值和层级，包括未知字段；不按字段名猜测含义，也不使用结构报告的条目采样限制。范围内的鉴权/SSO 字段若有值也会保留，因此应先自行筛选必要片段再分享。普通 `--resource-debug` 和 YAML `debug` 不启用原值导出；详见 [资源诊断说明](resource-diagnostics.md)。

诊断默认关闭；数组/同名 XML 节点最多取 8 项，遍历最多 512 节点、8 层、128 种字段，达到边界时标记 `truncated`。异常或动态字段名会被过滤并标记 `filtered_names`。这是字段结构样本，不能据此断言其他资源或接口没有名称。文件使用私有权限并在下次采集时原子替换；不开启时不会产生新文件，也不会自动删除旧报告。分享前检查采集时间和字段名，只提供这个文件即可，不需要提交配置、状态会话或完整日志。

退出码：0 成功/正常停止；2 配置/监听冲突；3 需要人工认证；4 认证拒绝；5 连接失败；6 状态/凭据错误。初次登录失败不自动重试密码；已有会话恢复耗尽后退出。外部进程管理器不要无限重启。
