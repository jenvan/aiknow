package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	// 子命令模式
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hashpwd":
			hashpwd()
			return
		case "version":
			fmt.Println(version)
			return
		case "help", "--help", "-h":
			printHelp()
			return
		}
	}

	// 默认启动服务器
	fmt.Printf("AiKnow Server v%s\n", version)
	fmt.Println("启动中...（占位实现）")
}

func printHelp() {
	fmt.Printf(`AiKnow - AI 知识库管理工具 v%s

用法:
  aiknow-server [命令] [参数]

命令:
  hashpwd [密码]    生成密码哈希
  version           显示版本信息
  help              显示此帮助信息
`, version)
}

func hashpwd() {
	var password string
	flag.Parse()

	if flag.NArg() > 0 {
		password = flag.Arg(0)
	} else {
		fmt.Print("请输入密码: ")
		fmt.Scanf("%s", &password)
	}

	if password == "" {
		fmt.Println("错误：密码不能为空")
		os.Exit(1)
	}

	// 占位：实际实现会在后续任务中完成
	fmt.Printf("密码哈希（占位，将在 FND-1-06 中实现）: [REDACTED_SK_KEY]\n")
}
