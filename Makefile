# AiKnow 构建与开发命令

# 版本信息（从 git tag 或默认值获取）
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# 编译目标二进制名称
BINARY_NAME = aiknow-server

.PHONY: build run test test-coverage vet fmt migrate-up clean

# 编译 cmd/aiknow-server/main.go，生成 bin/aiknow-server
build:
	go build -ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT)" \
		-o bin/$(BINARY_NAME) ./cmd/aiknow-server

# 编译并运行
run: build
	./bin/$(BINARY_NAME)

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

# 执行数据库迁移（占位，后续实现）
migrate-up:
	@echo "TODO: 实现数据库迁移执行器"
	@echo "迁移文件位于 migrations/ 目录"

# 清理编译产物
clean:
	rm -rf bin/ coverage.out coverage.html
