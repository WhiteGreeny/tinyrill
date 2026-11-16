# 多客户与 mihomo

原生或容器都先提供三组独立端口：A 21080/21081，B 31080/31081，C 41080/41081。直接验证可用后，再把 `examples/mihomo.yaml` 中的代理与规则片段合入现有配置，保留现有最终规则；示例不是覆盖用的完整 mihomo 配置。

SOCKS5 出站禁用 UDP；tinyrill 只接受 TCP CONNECT。应用直连该入口时用 `socks5h://127.0.0.1:21080` 传递域名。HTTP 入口保留 URL/CONNECT 主机名。上层若先用公共 DNS 解析，内部域名可能已泄露或失去客户身份，因此需检查现有 mihomo DNS/fake-IP 与规则行为；这里不创建公共 DNS 监听或替用户改变系统 DNS。

网关及必要认证端点必须在上层绕过当前 VPN 代理，尤其在 mihomo TUN/全局接管模式下，避免控制连接进入自身入口。示例只有占位网关名，实际 OAuth/CAS 域名与网关 IP 也需按部署情况排除。明确选择 DIRECT 的是控制端点；进入 tinyrill 的业务请求依然不会直连兜底。

不同客户都含 10.0.0.10 或相同业务域名时，目的地址不足以决定客户。通过独立应用代理、入口或人工 select 组区分；切换选择只影响新连接，应用应断开旧连接后重连。不要把多个客户 DNS 缓存混合，也不要用 failover/url-test 在客户之间自动切换。

规则支持的配置语义参见 [mihomo SOCKS 出站](https://wiki.metacubex.one/config/proxies/socks/) 和 [规则](https://wiki.metacubex.one/config/rules/)。候选版不承诺任意上层 TUN/UDP 行为，实际验收应同时记录上层配置和代理入口。

## IP 可以访问、经 mihomo 却失败

2026-11 的一次排障中，直接通过 tinyrill SOCKS 访问 IP 得到正常 HTTP 302，而经上层代理访问同一 IP 得到 `Empty reply from server`。上层实际传给 SOCKS 的是 DNS 缓存中对应的域名；直接把这个域名传给 tinyrill 也解析失败。域名和 IP 不是同一种解析输入，资源名称和空备注不影响此行为。下列地址、端口和域名均为匿名示例。

先在 Windows PowerShell 使用 **tinyrill 自己的端口**，以同一种 GET 请求比较 IP 和域名；替换为实际目标，避免把 HEAD 和 GET 的差异混入判断：

```powershell
curl.exe -q --proxy socks5h://127.0.0.1:21080 --noproxy localhost --connect-timeout 10 --max-time 20 -v -o NUL http://10.0.0.10:8080/
curl.exe -q --proxy socks5h://127.0.0.1:21080 --noproxy localhost --connect-timeout 10 --max-time 20 -v -o NUL http://portal.10.0.0.10.nip.io:8080/
```

然后用上层代理的端口重复相同请求。RC5 普通日志中的 `stage=dns` 表示解析失败，`stage=authorization` 表示授权拒绝，`stage=dial` 表示 VPN 业务连接建立失败。打开客户配置的 `debug: true`，重启该实例，可以看到 SOCKS 收到的域名、HTTP/SOCKS 拨号目标及 DNS 结果来源。上层是否显示请求与其日志级别、连接页面和 DNS 处理有关，不能只凭“没有日志”认定请求没有经过它。

如果已确认目标 IP 属于当前客户且应固定解析，从 RC5 起可在 **tinyrill 的客户配置** 中加入：

```yaml
dns:
  mode: vpn
  hosts:
    "+.10.0.0.10.nip.io": "10.0.0.10"
```

重启该客户实例后，基域名、`portal.10.0.0.10.nip.io` 和更多层子域名都命中同一 IP；其他 IP 的 nip.io 名称不受影响。单个名称也可用精确键。此配置只负责解析，资源授权和 VPN 通道仍必须允许目标，HTTP Host/TLS 域名保持应用原始值。它不会解析任意 nip.io 文本中的数字，也不会查询公共 DNS。

另一种部署选择是在 **mihomo 的顶层配置** 中合并下面的 `hosts`，并按现有配置管理方式应用；其作用域是上层代理，不能区分使用同名域名的多个客户：

```yaml
hosts:
  "+.10.0.0.10.nip.io": 10.0.0.10
```

本次排障在隔离的 Mihomo Meta v1.19.29 中分别验证了精确规则、基域名及多层后缀，用户也确认上层规则解决了原问题。tinyrill 的新映射有独立自动化测试，不把此前上层配置的实测当作 RC5 客户验收。官方客户端对同一业务可用，但没有采集它的实际 DNS/路由过程，因此不推断它一定采用某个特定解析器。

临时排障脚本、原始配置副本、缓存及含客户地址的日志已从工作区清理；仅保留本匿名说明。运行中的客户配置、会话和现有发布归档不在本次清理范围内。
