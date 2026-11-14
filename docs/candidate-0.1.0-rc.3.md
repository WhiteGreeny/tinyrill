# 0.1.0-rc.3 交付记录

日期：2026-11-14。程序与对应源码固定于 `4b25fe2b6e9c607018c323bac8350e3a41889005`；后续提交补充本次交付记录。Windows/Linux amd64 原生包、验证附件、源码与 rootless Podman 镜像已在本地生成并验证。

## 资源名称

用户提供的真实 EC 结构样本（采集时间 `2026-11-14T04:20:24Z`）确认存在 `Resource/Rcs/Rc/@name`。RC3 保留该属性，并在已有资源列表末尾添加 `NAME` 列：

- 同一资源展开出的域名、单个 IP 和 IP 范围保留同一名称。XML 实体正常解码，不额外猜测 URL 编码。
- 相同地址/协议/端口但名称不同的条目分别展示；空名称显示 `-`。去重仅用于展示，名称不参与授权匹配。
- 名称过滤终端控制字符，最多展示前 256 个字符，超长追加 `...`；stdout 继续只输出 JSON 状态。
- 默认显示前 20 条，`--resources full` 显示全部，`--resources none` 隐藏。无需开启 debug 即可展示名称。
- 包含此前诊断构建的 `--resource-debug`。诊断中的 `truncated: true` 只表示字段采样不完整，不会按该采样上限截断实际资源列表。

样本省略了字段值，因此不据此保证每条资源名称非空，也不推断网关版本。aTrust 名称尚待真实字段样本确认，目前名称列仍为 `-`。本轮未连接实际客户网关，实际名称显示效果待用户确认；此前用户验收结果仍按 [验收记录](acceptance.md) 保存。

## 交付文件

目录为 `dist/0.1.0-rc.3/`。目录内 `DELIVERY.md` 提供入口，`verification.json` 记录验证结果，`SHA256SUMS` 校验顶层文件；每个归档另有 `.sha256`。

| 文件 | 字节数 | 用途 |
|---|---:|---|
| `tinyrill-0.1.0-rc.3-windows-amd64.zip` | 7,032,073 | Windows 运行包 |
| `tinyrill-0.1.0-rc.3-linux-amd64.tar.gz` | 6,956,496 | Linux 运行包 |
| `tinyrill-0.1.0-rc.3-windows-amd64-verification.zip` | 270,433 | Windows 验证附件 |
| `tinyrill-0.1.0-rc.3-linux-amd64-verification.tar.gz` | 236,478 | Linux 验证附件 |
| `tinyrill-0.1.0-rc.3-source.tar.gz` | 247,525 | 对应源码 |
| `tinyrill-0.1.0-rc.3-linux-amd64-image.tar` | 21,125,632 | rootless Podman 镜像 |

运行包仍为 7 个文件：程序、两份配置示例、README、LICENSE、合并许可声明和 SHA256SUMS。主要归档 SHA256：

```text
3ea51a3c79b89a146c2bc8be03e41226cfa17cc3ba287baa3aa8f4ac7fe7a8eb  tinyrill-0.1.0-rc.3-windows-amd64.zip
012889868165011555b2ca9ddb5e2da5f78c0f091c8877e0bbbf67914b06bc26  tinyrill-0.1.0-rc.3-linux-amd64.tar.gz
b146c18bea685fc1c32d6d72f69c562e5a54fc58f36035976f865a61547b44a3  tinyrill-0.1.0-rc.3-source.tar.gz
d91cfca28922523fc709445f087d73bfcaca5b54aa2b123de8fe6e154859fb7a  tinyrill-0.1.0-rc.3-linux-amd64-image.tar
```

## 验证结果

- Windows 全量测试通过：WSL Go 1.27.4 交叉编译后在 Windows 实际执行。Linux 全量 race 与 go vet 通过。
- 名称相关测试覆盖中文/XML 实体、多地址展开、缺失名称、同地址不同名称、去重和稳定排序、终端控制字符、长度限制、stdout 隔离与授权规则不变。
- 诊断仍省略名称和地址的原值；超出诊断采样上限的资源及其名称完整保留，Windows/Linux 对应测试通过。
- 两平台实际归档、内部校验清单、源码提交、provenance、二进制与许可原件逐字节核对通过；Windows 解压运行、Linux 原生文件系统解压权限 0755、版本和两份配置检查通过。
- 两平台源码/实际二进制 govulncheck v1.9.0 均为 0 条发现；实际镜像归档 Trivy 0.76.2 全部严重性扫描为 0 条。结果仅对应本次目标与扫描时间。
- rootless Podman 非 root、只读根、cap-drop=all、配置读取与私有状态写入通过。镜像重新载入和受限启动通过，镜像内程序/许可声明与 Linux 运行包一致。

镜像 ID：`0a2cd7ad15b7e476d7463f52dced57428792cad05b1b066bafe7ce1a1c81cec5`。执行 `podman load -i tinyrill-0.1.0-rc.3-linux-amd64-image.tar` 后使用 `localhost/tinyrill:0.1.0-rc.3` 标签，运行参数见验证附件的 `docs/docker.md`。

## 更新

停止旧进程，将新运行包解压到独立目录，继续使用自己的 YAML 配置与状态目录。Windows 示例：`./tinyrill.exe run --config /path/to/customer.yaml --resources full`。Linux 使用 `./tinyrill`。无需重新配置凭据或开启普通 debug。

旧 RC2 和诊断包归档保留。当前仅本地交付，没有推送 Git、创建公开 release 或上传镜像。
