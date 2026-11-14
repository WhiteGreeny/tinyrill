# 0.1.0-rc.4 交付记录

日期：2026-11-14。程序与对应源码固定于 `f29d65f7ef671dd5c97f30f05da1c3397122cc42`；后续提交补充交付记录。Windows/Linux amd64 原生包、验证附件、源码与 rootless Podman 镜像已在本地生成并验证。

## 资源说明

根据用户确认，保留以下字段，分别展示所有有值内容：

| 协议 | 网关字段 | 展示标签 |
|---|---|---|
| aTrust | `data.appList.data.appInfo[].name` | `Group name` |
| aTrust | `data.appList.data.appInfo[].description` | `Group description` |
| aTrust | `data.appList.data.appInfo[].apps[].name` | `Name` |
| aTrust | `data.appList.data.appInfo[].apps[].description` | `Description` |
| EC | `Resource/Rcs/Rc/@name` | `Name` |
| EC | `Resource/Rcs/Rc/@note` | `Note` |

名称为空时，非空描述或备注直接显示。名称、描述、备注分别保存在模型中，文字相同也不互相覆盖；aTrust 分组说明随每个应用的资源一起保留。字段继承覆盖域名、单个 IP、IP 范围、CIDR 及网关 DNS 对应的 IP。

末列改为 `RESOURCE`，按字段分行显示，避免多个中文说明列错位。默认仍展示前 20 个条目，`--resources full` 展示全部。相同地址/协议/端口但说明字段不同的条目分别保留，去重不改动路由规则。每个显示字段过滤控制字符、修剪首尾空白，最多显示前 256 个字符；模型及原值诊断保留完整文本。

文本不参与授权或隧道选择；aTrust 的可选说明字段若为意外的非文本类型，忽略该说明而不拒绝其他有效授权数据。原有 AppID、NodeGroupID、TCP/L3 和 AddrPretend 元数据保持不变。

RC4 同时包含此前诊断构建的 `--resource-debug-values`，供人工查看完整资源分支；该开关及结构诊断均默认关闭，正常展示无需 debug。原值文件格式和边界见 [诊断说明](resource-diagnostics.md)。

## 交付文件

目录为 `dist/0.1.0-rc.4/`，包含入口 `DELIVERY.md`、验证结果 `verification.json` 和顶层文件校验清单 `SHA256SUMS`。每个归档另有 `.sha256`。

| 文件 | 字节数 | 用途 |
|---|---:|---|
| `tinyrill-0.1.0-rc.4-windows-amd64.zip` | 7,044,489 | Windows 运行包 |
| `tinyrill-0.1.0-rc.4-linux-amd64.tar.gz` | 6,970,672 | Linux 运行包 |
| `tinyrill-0.1.0-rc.4-windows-amd64-verification.zip` | 277,119 | Windows 验证附件 |
| `tinyrill-0.1.0-rc.4-linux-amd64-verification.tar.gz` | 240,734 | Linux 验证附件 |
| `tinyrill-0.1.0-rc.4-source.tar.gz` | 258,124 | 对应源码 |
| `tinyrill-0.1.0-rc.4-linux-amd64-image.tar` | 21,154,304 | rootless Podman 镜像 |

运行包仍为 7 个文件。主要归档 SHA256：

```text
e775bd4fd030c5eae7401058a021569d2c32a2abbe6e3a0bfe6f72e817b69b8c  tinyrill-0.1.0-rc.4-windows-amd64.zip
dbaff7995e3a9b55f18f178e34abf6fd1d521ec295bb5f72f32dc424d4a9f840  tinyrill-0.1.0-rc.4-linux-amd64.tar.gz
4316ca6f222033ae394dfb3cbe1b4ee64608c38119357534c49c0fe23b290e95  tinyrill-0.1.0-rc.4-source.tar.gz
eea94c725fa7027f9a39eccabc5ba75c331fd96640e09c37af5f24142ee99a0f  tinyrill-0.1.0-rc.4-linux-amd64-image.tar
```

## 验证结果

- Windows 全量回归通过：WSL Go 1.27.4 交叉编译后在 Windows 实际执行。Linux EC/aTrust/实例/IP 资源索引的 race 回归及全库 go vet 通过。
- 测试覆盖全部地址解析分支、空名称有描述、分组继承、同文字不同字段、说明差异参与展示去重、稳定排序、控制字符、非文本可选字段、授权与隧道元数据不变。合成终端输出已检查，stdout 继续保持 JSON 状态隔离。
- 两平台实际归档的内部校验、源码提交、provenance、二进制及许可原件核对通过。Windows 解压运行和配置检查、Linux 原生文件系统解压后的 0755 权限、版本/帮助/配置检查通过。
- Windows/Linux 源码与实际二进制 govulncheck v1.9.0 均为 0 条发现。最终镜像使用本次更新的 Trivy 漏洞库扫描，Trivy 0.76.2 的全部严重性系统与 Go 组件发现数为 0；结果仅对应本次目标和扫描时间。
- rootless Podman 非 root、只读根、cap-drop=all、配置读取与私有状态写入通过。镜像归档重新载入、受限启动和版本检查通过；镜像内程序及许可声明与 Linux 运行包一致。

镜像 ID：`8b37ad098e282e85aab96b75ed0e816ffc54af77fcaada32f1ac171c5bb99f99`。执行 `podman load -i tinyrill-0.1.0-rc.4-linux-amd64-image.tar` 后使用 `localhost/tinyrill:0.1.0-rc.4`，运行参数见验证附件中的 `docs/docker.md`。

## 更新与验收

停止旧进程，将运行包解压到独立目录，继续使用自己的 YAML、状态与凭据。Windows 示例为 `./tinyrill.exe run --config /path/to/customer.yaml --resources full`，Linux 使用 `./tinyrill`。原 RC3 和诊断包保留。

用户已确认字段需求和名称为空但描述非空的情况；本轮未连接实际客户网关，RC4 实际显示效果待用户确认。此前业务验收按 [验收记录](acceptance.md) 保存，不扩展到未报告的网关版本或测试时长。当前仅本地交付，没有推送 Git、创建公开 release 或上传镜像。
