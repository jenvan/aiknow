# AiKnow - AI 知识库管理工具

AiKnow 是一个面向企业内部的知识库管理工具，提供 AI 智能处理、混合检索、MCP 接入等核心功能。

## 特性

- ✅ 文章增删改查 + 手动/URL 采集录入
- ✅ AI 异步处理（摘要/分类/标签），状态可查询
- ✅ 混合检索（向量 + BM25 + RRF + Rerank）+ 关键词检索
- ✅ RESTful API + API Key 鉴权 + 速率限制
- ✅ MCP 四个工具（3 只读 + 1 写入，按 scope 过滤）
- ✅ MCP 请求日志 + 调试代理
- ✅ Web 管理后台（仪表盘/文章列表/编辑/录入/检索测试/MCP 调试/配置管理）
- ✅ 三层配置体系（L1 启动 / L2 业务热生效 / L3 凭据）
- ✅ 基础安全措施（防 SSRF、防 XSS）

## 技术栈

### 后端
- **语言**：Go 1.23+
- **框架**：Gin 1.10+
- **数据库**：PostgreSQL 17
- **扩展**：pgvector、pg_textsearch、zhparser、PGMQ

### 前端
- **框架**：Alpine.js 3.14+
- **CSS**：Tailwind CSS 3.4+
- **架构**：前后端分离

### AI 服务
- **LLM**：阿里云 Qwen 系列
- **Embedding**：BAAI bge-large-zh-v1.5 (1024 维)
- **Reranker**：BAAI bge-reranker-v2-m3

## 快速开始

### 环境要求
- Go 1.23+
- PostgreSQL 17
- Docker（可选，用于快速部署）

### 安装

```bash
# 克隆项目
git clone https://github.com/aiknow-org/aiknow.git
cd aiknow

# 安装依赖
go mod tidy

# 配置环境变量
cp .env.example .env
# 编辑 .env 文件，设置必要的环境变量

# 编译
make build

# 运行
make run
```

### 环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `DATABASE_URL` | ✅ | PostgreSQL 连接串 |
| `QWEN_API_KEY` | ✅ | Qwen API 密钥 |
| `BAAI_API_KEY` | ✅ | BAAI API 密钥 |
| `SESSION_SECRET` | ✅ | Session 签名密钥 |
| `ADMIN_PASSWORD_HASH` | ✅ | 管理员密码哈希 |

生成密码哈希：
```bash
aiknow-server hashpwd your_password
```

## API 文档

- REST API：见 [docs/API.md](docs/API.md)
- MCP 协议：见 [docs/MCP.md](docs/MCP.md)

## 部署

### Docker 部署

```bash
# 构建镜像
docker build -t aiknow .

# 运行容器
docker run -d \
  -p 8080:8080 \
  -e DATABASE_URL=postgres://user:pass@host:5432/aiknow \
  -e QWEN_API_KEY=your_key \
  -e BAAI_API_KEY=your_key \
  -e SESSION_SECRET=your_secret \
  -e ADMIN_PASSWORD_HASH=your_hash \
  aiknow
```

### 直接部署

```bash
# 编译
go build -o aiknow-server ./cmd/aiknow-server

# 运行
./aiknow-server serve
```

## 开发

### 构建命令

```bash
make build        # 编译二进制
make run          # 编译并运行
make test         # 运行测试
make vet          # 代码检查
make fmt          # 格式化代码
make clean        # 清理编译产物
```

### 数据库迁移

```bash
# 执行迁移
make migrate-up
```

## 项目结构

```
aiknow/
├── cmd/aiknow-server/      # 服务入口
├── internal/
│   ├── handler/            # HTTP Handler
│   │   ├── api/            # 对外 API
│   │   ├── admin/          # 后台管理 API
│   │   ├── web/            # Web UI 路由
│   │   └── mcp/            # MCP Server
│   ├── service/            # 业务逻辑
│   ├── repository/         # 数据访问
│   ├── model/              # 数据模型
│   └── config/             # 配置加载
├── pkg/                    # 公共库
├── migrations/             # 数据库迁移
├── web/                    # 前端静态资源
└── docs/                   # 项目文档
```

## 许可证

MIT License
