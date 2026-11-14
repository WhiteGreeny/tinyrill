# 资源名称字段诊断

诊断构建 `0.1.0-rc.2-diagnostic.1` 增加 `--resource-debug`。原 RC2 运行包没有这个开关。本轮先确认真实网关返回的名称字段，再据此实现名称展示；普通 `debug: true` 只打印已解析的资源，无法查看被忽略的字段。

使用诊断构建、继续引用自己的客户 YAML，无需修改密码或开启普通 debug：

```powershell
./tinyrill.exe run --config /path/to/customer.yaml --resource-debug
```

```sh
./tinyrill run --config /path/to/customer.yaml --resource-debug
```

取得资源后，终端会输出 `Resource field diagnostic saved (values omitted): ...` 和实际文件路径。文件名为 `resource-fields.json`，位于配置中的 `session.state_dir`；默认是配置目录下的 `state/<客户ID>/`。看到本次保存提示后，再检查该文件的 `captured_at`，确认不是此前留下的报告。

文件只保存资源字段路径、类型、采集时间和采样状态，不保存字段的原值。示意字段可能是 XML 的 `Resource/Rcs/Rc/@name` 或 JSON 的 `data.appList.data.appInfo[].apps[].name`；这些只是示例，不代表已确认实际网关使用这些名称。

EC 采集范围是 `Resource/Rcs`；aTrust 是 `data.appList.data.appInfo`。诊断受节点数、深度、字段数和采样数量限制；`truncated: true` 表示样本不完整，`filtered_names: true` 表示某些字段名未导出，`scope_found: false` 表示没有取得预期资源分支。不能凭一个不完整样本断言整个网关不提供资源名称。

确认文件内容后，仅提供这个 JSON 即可；若知道网关版本，可一起注明。不需要提供客户 YAML、完整日志、会话文件、认证响应或真实资源名称。字段名也来自网关，分享前仍应快速检查。

该开关默认关闭；不修改资源访问规则，不增加网关请求。报告使用私有文件权限，下次采集会覆盖旧报告；报告写入失败会提示，VPN 初始化继续。停止使用开关后不会生成新报告，已有报告可自行删除。

## 本地诊断包

2026-11-14 已生成 Windows/Linux amd64 诊断包，位于 `dist/0.1.0-rc.2-diagnostic.1/`。程序与对应源码固定于 `aaf9f075633d21bf8d5c950480f3abff69d61e39`；目录中的 `DELIVERY.md` 提供运行包、源码、验证附件和校验入口。原 RC2 产物保留。

Windows 实际执行的相关回归、Linux 相关 race 测试、全库 vet 通过，覆盖两个协议的字段采集、原值遗漏、采样边界、路由规则不变、默认关闭和报告原子替换。两平台归档校验、版本/帮助/示例配置及源码对应关系通过，源码与实际二进制 govulncheck 均为 0 条发现。本次仅交付原生诊断包，没有重新构建容器镜像，也尚未取得真实客户的字段样本。

```text
d3616d88c190bf18a7808245a90047bfc1ba5a02ec4de0cca2e0b5627e8bd9b2  tinyrill-0.1.0-rc.2-diagnostic.1-windows-amd64.zip
2edb9e212009af2a7ea95d99b2654ffdf529e20f09d2f26b82bb71ab0cc758ee  tinyrill-0.1.0-rc.2-diagnostic.1-linux-amd64.tar.gz
```
