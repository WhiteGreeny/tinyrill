# 资源名称字段诊断

诊断构建 `0.1.0-rc.2-diagnostic.1` 增加 `--resource-debug`。原 RC2 运行包没有这个开关。普通 `debug: true` 只打印已解析的资源，无法查看被忽略的字段。

2026-11-14，用户提供的真实 EC 字段样本（采集时间 `2026-11-14T04:20:24Z`）确认存在 `Resource/Rcs/Rc/@name` 属性，RC3 据此接入名称展示。样本的 `truncated: true` 只表明诊断采样被截断；字段值被省略，因此不能据此保证名称非空或判断业务含义。

随后提供的 aTrust 样本（采集时间 `2026-11-14T04:26:08Z`）同时包含 `appInfo[].name`、`appInfo[].workspaceName`、`appInfo[].apps[].name` 和两层 `description`。之后用户明确要求保留分组及应用两层的 `name` 和 `description`，并指出名称可能为空而描述有值；EC 另需保留 `note`。RC4 按此确认接入四个 aTrust 字段和 EC 的名称/备注，分别展示所有有值字段。`workspaceName` 继续保留在原值导出中，未新增到普通资源列表。

## 导出实际资源值

`0.1.0-rc.3-diagnostic.1` 增加独立开关 `--resource-debug-values`。原 RC3 和旧诊断包没有这个开关。继续使用自己的客户配置，无需设置 YAML 的 `debug`：

```powershell
./tinyrill.exe run --config /path/to/customer.yaml --resource-debug-values
```

Linux 使用 `./tinyrill`。该开关单独使用即可，同时生成原有结构报告和含实际值的资源文件。文件位于客户 `session.state_dir`；终端打印保存路径，内容和解析错误原文不进入日志或 JSON 状态。

| 协议 | 原值文件 | 内容 |
|---|---|---|
| aTrust | `resource-values.json` | `resources` 保存完整的 `data.appList.data.appInfo` 数组，包括分组、工作空间、应用、名称、描述、地址及未知字段，保留层级、类型与数值精度 |
| EC | `resource-values.xml` | `ResourceDiagnostic` 元数据节点内保存完整的 `Rcs` 子树，包括所有 `Rc` 属性、子元素和文本；不重新缩进 XML，以保留文本空格 |

两种文件都带有协议、原始范围、采集时间、`scope_found`、`values_omitted=false` 和 `truncated=false`。原值导出不按名称猜测字段、不使用结构报告的前 8 项采样限制，也不截断字段值；因此结构报告可以是 `truncated=true`，原值文件仍保留整个资源分支。较大或深层 JSON 使用紧凑格式，内容完整。

这是含实际客户资源信息的本地文件；范围内的鉴权或 SSO 配置字段若有值，也会保留。范围外的认证响应、会话数据和请求头不导出。请先自行筛选，只提供用于解释字段的必要片段，或直接说明哪一层字段对应资源名称；可以将实际名称和地址替换为示例，保留字段名及层级。

文件使用 Linux 0600 / Windows 私有 ACL，原子替换旧文件，并由 `.gitignore` 排除。诊断失败不改变连接流程，也不会写入半份原值文件；失败时旧文件可能保留，以本次保存提示和 `captured_at` 为准。原值开关默认关闭，普通 debug 和 `--resource-debug` 均不会启用它；只有显式添加 `--resource-debug-values` 才会生成原值文件，停止使用后已有文件可自行删除。

## 仅导出字段结构

RC3 已包含 `--resource-debug` 结构诊断开关，正常展示 EC 名称无需开启。需要继续采集字段时，使用 RC3 或原诊断构建，继续引用自己的客户 YAML：

```powershell
./tinyrill.exe run --config /path/to/customer.yaml --resource-debug
```

```sh
./tinyrill run --config /path/to/customer.yaml --resource-debug
```

取得资源后，终端会输出 `Resource field diagnostic saved (values omitted): ...` 和实际文件路径。文件名为 `resource-fields.json`，位于配置中的 `session.state_dir`；默认是配置目录下的 `state/<客户ID>/`。看到本次保存提示后，再检查该文件的 `captured_at`，确认不是此前留下的报告。

文件只保存资源字段路径、类型、采集时间和采样状态，不保存字段的原值。EC 与 aTrust 的字段结构已由用户样本确认，当前展示字段及层级按用户后续确认接入，见 [资源展示映射](configuration.md)。

EC 采集范围是 `Resource/Rcs`；aTrust 是 `data.appList.data.appInfo`。诊断受节点数、深度、字段数和采样数量限制；`truncated: true` 表示样本不完整，`filtered_names: true` 表示某些字段名未导出，`scope_found: false` 表示没有取得预期资源分支。不能凭一个不完整样本断言整个网关不提供资源名称。诊断采样上限不截断实际资源解析，已用超出采样数量的命名资源测试确认。

确认文件内容后，仅提供这个 JSON 即可；若知道网关版本，可一起注明。不需要提供客户 YAML、完整日志、会话文件、认证响应或真实资源名称。字段名也来自网关，分享前仍应快速检查。

该开关默认关闭；不修改资源访问规则，不增加网关请求。报告使用私有文件权限，下次采集会覆盖旧报告；报告写入失败会提示，VPN 初始化继续。停止使用开关后不会生成新报告，已有报告可自行删除。

## 原值诊断包交付

2026-11-14，已生成 `0.1.0-rc.3-diagnostic.1` Windows/Linux amd64 原生包，目录为 `dist/0.1.0-rc.3-diagnostic.1/`，程序与对应源码固定于 `cb12a46fbd016bf176659276fef995021f7376af`。目录内 `DELIVERY.md` 提供运行包、验证附件和源码入口；`verification.json` 与 `SHA256SUMS` 保存验证结果及整体校验。

Windows 全量回归、Linux 全量 race/vet 通过，覆盖完整资源原值、未知字段、分组/应用关系、数值精度、大型/深层资源、私有权限、原子覆盖、写入失败、默认关闭、日志/状态隔离和授权规则保持。两平台归档、源码对应关系、许可原件、版本/帮助/示例配置检查通过，源码及实际二进制 govulncheck 均为 0 条发现。本次没有重建容器镜像；实际资源值仍由用户在客户环境导出并筛选。

```text
1ac81358bbda62a07025e4c3793c1126e95149bbdceb73b8b239194298875e08  tinyrill-0.1.0-rc.3-diagnostic.1-windows-amd64.zip
57bbc3c72aaa3194876852d9fdad6559584a2a1e2ac4d0a45bd1e1eba1660850  tinyrill-0.1.0-rc.3-diagnostic.1-linux-amd64.tar.gz
```

## 早期字段结构诊断包

2026-11-14 已生成 Windows/Linux amd64 诊断包，位于 `dist/0.1.0-rc.2-diagnostic.1/`。程序与对应源码固定于 `aaf9f075633d21bf8d5c950480f3abff69d61e39`；目录中的 `DELIVERY.md` 提供运行包、源码、验证附件和校验入口。原 RC2 产物保留。

Windows 实际执行的相关回归、Linux 相关 race 测试、全库 vet 通过，覆盖两个协议的字段采集、原值遗漏、采样边界、路由规则不变、默认关闭和报告原子替换。两平台归档校验、版本/帮助/示例配置及源码对应关系通过，源码与实际二进制 govulncheck 均为 0 条发现。上述诊断包只交付原生构建，没有重新构建容器镜像；交付时尚未取得真实客户的字段样本。

```text
d3616d88c190bf18a7808245a90047bfc1ba5a02ec4de0cca2e0b5627e8bd9b2  tinyrill-0.1.0-rc.2-diagnostic.1-windows-amd64.zip
2edb9e212009af2a7ea95d99b2654ffdf529e20f09d2f26b82bb71ab0cc758ee  tinyrill-0.1.0-rc.2-diagnostic.1-linux-amd64.tar.gz
```
