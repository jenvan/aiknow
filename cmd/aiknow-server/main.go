// 包声明与导入
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/aiknow-org/aiknow/cmd/aiknow-server/hashpwd"
)

// 版本信息，由 ldflags 注入
var (
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
)

func main() {
	// 定义全局标志
	help := flag.Bool("help", false, "显示帮助信息")
	versionFlag := flag.Bool("version", false, "显示版本信息")
	flag.Parse()

	// --help 显示帮助
	if *help {
		printHelp()
		os.Exit(0)
	}

	// --version 显示版本
	if *versionFlag {
		fmt.Printf("aiknow-server %s (commit: %s, built: %s)\n", version, gitCommit, buildTime)
		os.Exit(0)
	}

	// 获取子命令
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "hashpwd":
			hashpwd.Main(args[1:])
			return
		case "version":
			fmt.Printf("aiknow-server %s (commit: %s, built: %s)\n", version, gitCommit, buildTime)
			return
		case "serve", "start", "":
			// 默认启动 HTTP 服务
		default:
			fmt.Fprintf(os.Stderr, "未知子命令: %s\n请使用 --help 查看帮助\n", args[0])
			os.Exit(1)
		}
	}

	// 启动 HTTP 服务器（暂为空实现）
	runServer()
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Println(`AiKnow - 企业内部知识库管理系统

用法:
  aiknow-server [选项] [子命令] [参数]

选项:
  --help      显示此帮助信息
  --version   显示版本信息

子命令:
  hashpwd     生成 bcrypt 密码哈希
              用法: aiknow-server hashpwd [密码]

  version     显示版本信息
              用法: aiknow-server version

  serve       启动 HTTP 服务（默认）
              用法: aiknow-server serve

示例:
  aiknow-server --help
  aiknow-server version
  aiknow-server hashpwd mysecretpassword
  aiknow-server serve`)
}

// runServer 启动 HTTP 服务器
func runServer() {
	// 初始化 Gin 在 release 模式
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// 健康检查端点
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "version": version})
	})

	// 启动服务器
	addr := ":8080"
	fmt.Printf("AiKnow server starting on %s (version: %s)\n", addr, version)

	go func() {
		if err := router.Run(addr); err != nil {
			fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
			os.Exit(1)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\n正在关闭服务器...")
}
