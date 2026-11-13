# 多客户与 mihomo

原生或容器都先提供三组独立端口：A 21080/21081，B 31080/31081，C 41080/41081。直接验证可用后，再把 `examples/mihomo.yaml` 中的代理与规则片段合入现有配置，保留现有最终规则；示例不是覆盖用的完整 mihomo 配置。

SOCKS5 出站禁用 UDP；tinyrill 只接受 TCP CONNECT。应用直连该入口时用 `socks5h://127.0.0.1:21080` 传递域名。HTTP 入口保留 URL/CONNECT 主机名。上层若先用公共 DNS 解析，内部域名可能已泄露或失去客户身份，因此需检查现有 mihomo DNS/fake-IP 与规则行为；这里不创建公共 DNS 监听或替用户改变系统 DNS。

网关及必要认证端点必须在上层绕过当前 VPN 代理，尤其在 mihomo TUN/全局接管模式下，避免控制连接进入自身入口。示例只有占位网关名，实际 OAuth/CAS 域名与网关 IP 也需按部署情况排除。明确选择 DIRECT 的是控制端点；进入 tinyrill 的业务请求依然不会直连兜底。

不同客户都含 10.0.0.10 或相同业务域名时，目的地址不足以决定客户。通过独立应用代理、入口或人工 select 组区分；切换选择只影响新连接，应用应断开旧连接后重连。不要把多个客户 DNS 缓存混合，也不要用 failover/url-test 在客户之间自动切换。

规则支持的配置语义参见 [mihomo SOCKS 出站](https://wiki.metacubex.one/config/proxies/socks/) 和 [规则](https://wiki.metacubex.one/config/rules/)。候选版不承诺任意上层 TUN/UDP 行为，实际验收应同时记录上层配置和代理入口。
