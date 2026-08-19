# MobaXterm KeyGen (Go)

一个用 Go 语言编写的 MobaXterm 授权文件生成器，复刻自 Python 版 KeyGen。
生成 `Custom.mxtpro`（内含 `Pro.key` 的 ZIP 容器），用于激活 MobaXterm 专业版。

## 原理简介

授权数据按如下格式构造明文：

```
<licenseType>#<userName>|<major><minor>#<count>#<major>3<minor>6<minor>#0#0#0#
```

例如用户 `admin`、版本 `26.3`、数量 `1` 时：

```
1#admin|263#1#26363#0#0#0#
```

随后经过：

1. **加密**：逐字节与密钥 `0x787` 异或（对称 `EncryptBytes`/`DecryptBytes`）。
2. **编码**：自定义 Base64 表 `ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=` 编码。
3. **打包**：将编码结果以 `STORED`（无压缩）方式写入名为 `Pro.key` 的 ZIP 文件，即 `Custom.mxtpro`。

> 关键细节：ZIP 必须预先填好 CRC32 与大小，并使用 `CreateRaw`，避免 Go 默认设置
> `general purpose bit 3`（data descriptor）。MobaXterm 26.x 不会解析带 data descriptor 的
> 条目，会导致激活失败。

## 目录结构

```
.
├── main.go          # 核心算法 + 内置 Web 服务
├── go.mod           # Go 模块定义
├── Makefile         # 构建脚本
└── README.md
```

## 构建与运行

需要本地安装 Go 1.21+。

```bash
# 方式一：使用 Makefile
make            # 构建当前平台二进制 MobaXtermKeyGen
make run        # 构建并直接运行
make clean      # 清理产物

# 方式二：直接使用 go 命令
go build -o MobaXtermKeyGen .
./MobaXtermKeyGen
```

运行后会在 `http://localhost:50000` 启动一个 Web 界面，并自动打开浏览器。

## 使用步骤

1. 在 Web 页面填写：
   - **用户名 (Username)**：激活后显示的名称，如 `admin`
   - **版本号 (Version)**：MobaXterm 主版本.次版本，如 `26.3`
   - **授权数量 (Count)**：并发/用户数量，如 `1`
2. 点击 **生成并下载**，浏览器会保存 `Custom.mxtpro` 文件。
3. 将 `Custom.mxtpro` 放到 MobaXterm 安装目录（与 `MobaXterm.exe` 同级目录）。
4. 重启 MobaXterm，在 **Help → Register** 中查看已激活的专业版授权。
5. 若未生效，请以**管理员身份**运行一次 MobaXterm 再重启。

也可直接通过 URL 传参生成：

```
http://localhost:50000/gen?name=admin&ver=26.3&count=1
```

## 命令行参数说明（代码层面）

`GenerateLicense(licenseType, count, userName, majorVersion, minorVersion)` 支持以下授权类型：

| 常量                   | 值 | 说明       |
| ---------------------- | -- | ---------- |
| `LicenseProfessional`  | 1  | 专业版     |
| `LicenseEducational`   | 3  | 教育版     |
| `LicensePersonal`      | 4  | 个人版     |

当前 Web 界面默认生成专业版（`LicenseProfessional`）。

## 免责声明

本项目仅供学习与研究软件授权机制之用。请遵守软件许可协议，支持正版软件。
