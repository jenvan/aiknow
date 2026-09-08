# AiKnow 构建与开发命令

# 版本信息（从 git tag 或默认值获取）
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# 编译目标二进制名称
BINARY_NAME = aiknow

.PHONY: build run test test-coverage vet fmt migrate-up clean docker-build up down logs ps

# 编译 cmd/aiknow-server，生成 bin/aiknow
build:
	go build -ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT)" \
		-o bin/$(BINARY_NAME) ./cmd/aiknow-server

# 编译并运行（serve 为必需子命令，缺省只会打印帮助）
run: build
	./bin/$(BINARY_NAME) serve

# 运行所有测试
test:
	go test -v -count=1 ./...

# 运行测试并生成覆盖率报告
test-coverage:
	go test -v -count=1 -coverprofile=coverage.out -coverpkg=./... ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成：coverage.html"

# 运行 go vet 检查代码质量问题
vet:
	go vet ./...

# 格式化代码
fmt:
	go fmt ./...

# 数据库迁移：服务启动时自动执行，无需单独命令
migrate-up:
	@echo "迁移在服务启动时自动执行（见 cmd/aiknow-server/main.go）"
	@echo "迁移文件：internal/repository/migrations/（已通过 go:embed 打包）"
	@echo "注意：扩展需手动创建，详见 docs/DATABASE_SETUP.md"

# 清理编译产物
clean:
	rm -rf bin/ coverage.out coverage.html

# ---- Docker ----

# 构建镜像
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t aiknow:$(VERSION) -t aiknow:latest .

# 启动全部服务（首次会自动构建镜像）
up:
	VERSION=$(VERSION) GIT_COMMIT=$(GIT_COMMIT) docker compose up -d --build

# 停止服务（保留数据卷）
down:
	docker compose down

# 查看应用日志
logs:
	docker compose logs -f app

# 查看服务状态
ps:
	docker compose ps
