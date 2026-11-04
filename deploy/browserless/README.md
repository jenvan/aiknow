# Browserless 独立部署说明

供 AiKnow 采集「无头浏览器渲染 + 全页截图」使用。独立部署在一台采集服务可
访问的机器上，与 AiKnow 主服务解耦、可独立扩缩容。AiKnow 通过 browserless 的
HTTP `/function` 端点（自定义 Puppeteer 代码）一次调用同时拿「渲染后 HTML +
全页截图」，不引入 chromedp 依赖。

## 目录

- `docker-compose.yml` — browserless 服务编排
- `.env.example` — 部署侧配置（令牌）

## 部署

1. 将本目录拷到目标服务器（需已装 Docker + docker compose）。
2. `cp .env.example .env`，设置 `TOKEN`：
   ```
   TOKEN=$(openssl rand -hex 16)
   ```
3. `docker compose up -d`
4. 健康检查：
   ```
   curl -H "x-access-token: $TOKEN" http://127.0.0.1:3000/health
   ```

## AiKnow 侧配置

在 AiKnow 的 `.env` 或 `config.yaml`（render 段）中：

```
COLLECTOR_BROWSER_ENABLED=true
COLLECTOR_BROWSER_ENDPOINT=http://<browserless机器IP>:3000
COLLECTOR_BROWSER_TOKEN=<同上 TOKEN>
COLLECTOR_BROWSER_TIMEOUT=30s
```

注意 `COLLECTOR_BROWSER_ENDPOINT` 是**指向本 compose 服务所在机器的地址**。本机测试用
`http://127.0.0.1:3000`；跨服务器用该机的内网 IP。

另需在 AiKnow `.env` 配置腾讯云 COS（截图归档），否则只采正文、不存截图：

```
COS_SECRET_ID=...
COS_SECRET_KEY=...
COS_BUCKET=aiknow-1300000000
COS_REGION=ap-guangzhou
COS_PATH_PREFIX=screenshots
```

浏览器渲染耗时较长，`COLLECTOR_BROWSER_TIMEOUT` 应不小于本服务的 `TIMEOUT`。

## 调参

| 变量 | 默认 | 说明 |
|---|---|---|
| `TIMEOUT` | 30000 | 单页硬超时（ms）。AiKnow 侧 `COLLECTOR_BROWSER_TIMEOUT` 应 >= 此值 |
| `CONCURRENT` | 3 | 并发渲染上限，按机器内存调。8C16G 建议 3-4 |
| `QUEUED` | 10 | 排队上限，超出快速失败 |
| `shm_size` | 1gb | Chromium 共享内存，渲染大页防崩 |

## 安全

- `TOKEN` 为空则不做鉴权——**仅**适用于内网强隔离环境。
- `ports` 绑定到 `127.0.0.1`，跨机访问需自行调整绑定地址或走反向代理，**勿直接暴露公网**。
- 浏览器内的页面 JS 可自发请求。AiKnow 侧对提交给浏览器的 URL 做过 SSRF 校验
  （`pkg/ssrf`），但仍需保证本服务本身处在一个访问不到敏感内网的位置（DMZ/独立网段）。
