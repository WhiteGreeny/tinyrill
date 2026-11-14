# 0.1.0-rc.2 交付记录

日期：2026-11-14。Windows/Linux amd64 运行包、验证附件、对应源码与 rootless Podman 镜像已在本地生成并验证。程序与源码固定于 `bf97632cea06989c8bd307c31907d100e588ff2d`；后续 Git 提交仅补充本次交付记录。没有推送 Git、创建公开 release 或上传镜像。

## 本轮变化

- 登录就绪和恢复连接后显示客户、代理监听地址、授权域名/IP 范围、协议和端口。默认最多 20 条；`--resources full` 显示全部，`--resources none` 关闭。稳定排序和去重仅用于展示，不改变路由；终端控制字符受到过滤，stdout 仍只输出 JSON 状态。
- UDP/ICMP 标为当前 TCP 代理不支持。列表来自已解析的授权资源，未主动探测可达性，不含业务系统名称。
- 运行包缩减为 7 个文件；Windows RC1 原包有 57 个文件。完整文档、验收脚本、扫描与构建记录独立为验证附件；同版本各平台共用源码归档。三类材料由提交号和 SHA256 关联。
- Go、模块依赖及仓库内 TCP 探测代码的原始许可声明合并成一个文件，验证包仍保留逐文件原件与来源索引。镜像携带相同声明。
- 移除未调用的 HTTP 探测器、URI/协议选择辅助代码、旧 DNS 接口及未引用图片，更新 Issue 模板。保留仍使用的 TCP 线路探测、VPN DNS、gVisor、诊断代码及许可证。`go mod tidy -diff` 无变化。
- 清理旧测试/构建残留约 281.79 MiB；工具缓存集中到忽略的 `_build/`。RC1 交付归档和最终验证记录保留，三个归档的 SHA256 与原记录一致。
- 发布脚本增加 govulncheck JSON 转换检查，使可达漏洞以非零退出码阻止打包；合成可达漏洞报告已验证退出码 3。

## 交付文件

目录为 `dist/0.1.0-rc.2/`，归档旁均有 `.sha256`。目录内的 `DELIVERY.md`、`verification.json` 和 `SHA256SUMS` 提供入口与整体校验。

| 文件 | 字节数 | 用途 |
|---|---:|---|
| `tinyrill-0.1.0-rc.2-windows-amd64.zip` | 7,012,269 | Windows 运行包 |
| `tinyrill-0.1.0-rc.2-linux-amd64.tar.gz` | 6,940,552 | Linux 运行包 |
| `tinyrill-0.1.0-rc.2-windows-amd64-verification.zip` | 262,905 | Windows 验证附件 |
| `tinyrill-0.1.0-rc.2-linux-amd64-verification.tar.gz` | 230,845 | Linux 验证附件 |
| `tinyrill-0.1.0-rc.2-source.tar.gz` | 236,425 | 对应源码与构建脚本 |
| `tinyrill-0.1.0-rc.2-linux-amd64-image.tar` | 21,084,672 | rootless Podman 镜像归档 |

运行包内只有主程序、两份 YAML 示例、README、LICENSE、THIRD_PARTY_NOTICES.txt 和 SHA256SUMS。实际运行读取主程序、所选 YAML 及配置明确引用的 CA/secret/证书等文件；无需加载扫描报告、源码或 Go 工具链。

主要归档 SHA256：

```text
165ff4044301685d5dc9ad8177e9dfdea3a35e537e02ad60cdefd20d9b2eb65e  tinyrill-0.1.0-rc.2-windows-amd64.zip
538d5200da97cf36ca6e5e83a4ebe7e228999aba5fcd1c38f8008a6a4f72269d  tinyrill-0.1.0-rc.2-linux-amd64.tar.gz
fc2fe5b34df3602b38293588837b49228ec32e76ade21ed2ee36e9bdb33a3216  tinyrill-0.1.0-rc.2-windows-amd64-verification.zip
b284cb26c853981730f41c694a4510617da4d4e5431ee0988e04ee5cc4ca7e30  tinyrill-0.1.0-rc.2-linux-amd64-verification.tar.gz
78331da91c602f5f347190a024f4e5e31bf260ab80cbe62de8a4ee561bf36fd0  tinyrill-0.1.0-rc.2-source.tar.gz
d6c8ef4edfe6725843275c03e118496bca96615f273d796f8d76e6237534dd0a  tinyrill-0.1.0-rc.2-linux-amd64-image.tar
```

## 验证结果

- Windows 全量测试通过：WSL Go 1.27.4 交叉编译，测试程序在 Windows 实际执行。未修改 Defender 设置。
- Linux 全量 race、go vet、模块校验和格式检查通过；Windows Credential Manager 与隔离 Linux Secret Service 的合成凭据增读改删通过。
- 资源展示测试覆盖摘要/完整/关闭、去重、稳定排序、空列表、UDP/ICMP 边界、控制字符过滤和 JSON 输出隔离。
- 两平台实际归档的全部文件校验通过；程序、源码归档、provenance 与对应扫描附件一致；逐依赖和内嵌代码原始许可声明在合并文件中逐字节保留。
- Windows 解压运行，Linux 原生文件系统解压后的 0755 权限、版本与两份配置检查通过。
- 两平台源码/实际二进制 govulncheck v1.9.0 均为 0 条发现；最终镜像归档的 Trivy 0.76.2 系统与 Go 组件扫描均为 0 条。报告只对应本次目标及扫描时间。
- rootless Podman 5.7.0 的非 root、只读根、cap-drop=all、配置读取与私有状态写入通过。导出归档重新载入并受限启动通过；标签与程序版本/源码提交相符，镜像程序和许可声明与 Linux 原生包完全一致。

镜像 ID：`0555f0dc1feb532ef74a303f1aca1c48ca7b8d6bb27aa89e9aab00414ba08111`。载入后标签为 `localhost/tinyrill:0.1.0-rc.2`：

```sh
podman load -i tinyrill-0.1.0-rc.2-linux-amd64-image.tar
```

使用验证附件中 `docs/docker.md` 的运行参数，将示例镜像标签改为上述 RC2 标签。镜像扫描报告为 `tinyrill-0.1.0-rc.2-image-scan.json`。

## 客户验收与更新

此前 Windows/Linux 用户实测结果仍按 [验收记录](acceptance.md) 保存。本轮未连接实际客户网关，不增加网关版本兼容声明；RC2 新增资源展示的实际网关效果由用户后续验收确认。

更新时停止旧进程，保留客户配置、状态与凭据，用独立目录解压新运行包；不要覆盖自定义 YAML。可继续使用原配置，添加 `--resources full` 查看完整列表。需要回退时使用保留的 RC1 归档。
