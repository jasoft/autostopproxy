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

## 许可证

MIT
