# AutoStopProxy

AutoStopProxy 是一个用 Go 编写的智能反向代理工具。它能够根据请求自动管理 Docker 容器的生命周期：当有请求进来时自动启动容器，并在长时间无访问时自动停止容器，从而节省系统资源。

特别适用于 GPU 密集型服务（如 PaddleX, OCR, LLM 等）或不常用的后台工具。

## 功能特性

- **按需启动**：检测到匹配路由的请求时，如果 Docker 容器未运行，则自动执行 `docker start`。
- **自动停机**：在指定的时间内（如 15 分钟）没有新请求，自动执行 `docker stop`。
- **健康检查等待**：支持配置健康检查路径（如 `/health`），确保容器内的应用完全启动并就绪后再转发请求，避免出现连接重置或 EOF 错误。
- **透明代理**：基于 Go 标准库实现的强健反向代理。

## 快速开始

### 安装

确保你已经安装了 Go 和 Docker。

```bash
git clone https://github.com/yourusername/autostopproxy.git
cd autostopproxy
go build -o autostopproxy.exe
```

### 使用示例

假设你有一个 Docker 容器名为 `paddlex`，它在启动后监听 `8081` 端口：

```powershell
.\autostopproxy.exe -route / -container paddlex -target http://localhost:8081 -listen :8080 -timeout 10m -health /health
```

现在访问 `http://localhost:8080`：

1. 如果 `paddlex` 容器未启动，代理会先启动它。
2. 代理会不断检查 `http://localhost:8081/health` 直到返回 200。
3. 随后你的请求会被成功转发。
4. 如果 10 分钟内没有新请求，`paddlex` 会自动停止。

## 命令行参数

| 参数 | 说明 | 默认值 |
| :--- | :--- | :--- |
| `-route` | 需要代理的路径前缀。使用 `/` 代理所有流量。 | `/ocr` |
| `-container` | 目标 Docker 容器的名称或 ID。 | `ocr-service` |
| `-target` | 容器启动后在宿主机上对应的服务地址。 | `http://localhost:8081` |
| `-listen` | 反向代理自身监听的地址和端口。 | `:8080` |
| `-timeout` | 空闲超时时间（如 `15m`, `1h`, `30s`）。 | `15m` |
| `-health` | 健康检查路径。如果设置，会等待该路径返回 200 再转发流量。 | (空) |

## 系统要求

- 运行环境需安装 Docker 且当前用户有权操作 Docker。
- Windows (win32) 或 Linux。

## Windows 服务安装 (NSSM)

如果你希望在 Windows 系统启动时自动运行 AutoStopProxy，可以使用 NSSM (Non-Sucking Service Manager) 来将其注册为 Windows 服务。

### NSSM 安装步骤

#### 1. 下载和安装 NSSM

从 [NSSM 官方网站](https://nssm.cc/)下载最新版本，解压到任意目录（如 `C:\nssm`）。

#### 2. 安装服务

以**管理员身份**打开 PowerShell 或 CMD，执行以下命令：

```powershell
# 进入 NSSM 目录（选择与你的系统位数对应的版本）
cd C:\nssm\win64  # 或 win32

# 安装服务（将 AutostopProxy 替换为你的服务名）
.\nssm install Autostopproxy "E:\Projects\autostopproxy\autostopproxy.exe" "-route / -container mycontainer -target http://localhost:8081 -listen :8080 -timeout 15m -health /health"
```

**参数说明：**

- 第一个参数：服务名称（不能有空格）
- 第二个参数：可执行文件的完整路径
- 第三个及之后的参数：程序的命令行参数

### 3. 修改服务设置

如需修改服务配置（如启动参数、工作目录、日志等），使用编辑命令：

```powershell
cd C:\nssm\win64
.\nssm edit Autostopproxy
```

会打开图形界面，你可以在其中修改：

- **Application 标签页**：应用程序路径和参数
- **Details 标签页**：显示名称、描述等信息
- **Log On 标签页**：运行服务的用户账户（通常选择 Local System 或指定用户）
- **I/O 标签页**：日志输出路径（用于调试）

### 4. 启动服务

```powershell
# 启动服务
.\nssm start Autostopproxy

# 停止服务
.\nssm stop Autostopproxy

# 重启服务
.\nssm restart Autostopproxy
```

也可以通过 Windows 服务管理器操作：

```powershell
# 打开服务管理器
services.msc
```

找到 "Autostopproxy" 服务，右键选择"启动"、"停止"或"重新启动"。

### 5. 卸载服务

如需移除服务：

```powershell
cd C:\nssm\win64
.\nssm remove Autostopproxy confirm
```

### 常见问题

**Q: 服务启动失败，如何查看错误日志？**

A: 在 NSSM 编辑界面的 "I/O" 标签页中设置日志输出路径，重启后查看日志文件。

**Q: 如何修改服务运行权限？**

A: 在 NSSM 编辑界面的 "Log On" 标签页中修改服务运行账户。如果应用需要访问 Docker，建议使用具有相应权限的用户账户。

**Q: 开机时服务没有自动启动？**

A: 确认服务的启动类型为"自动"，可在服务管理器中修改。

## 许可证

MIT
