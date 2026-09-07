// 包声明与导入
package hashpwd

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

// Main 是 hashpwd 子命令的入口函数
// 接收明文密码，输出 bcrypt 哈希值到 stdout
func Main(args []string) {
	var password string

	// 解析参数
	if len(args) > 0 {
		password = args[0]
	} else {
		// 从 stdin 读取密码
		fmt.Fprint(os.Stderr, "请输入密码（从 stdin 读取）: ")
		fmt.Scanln(&password)
	}

	if password == "" {
		fmt.Fprintf(os.Stderr, "错误: 密码不能为空\n")
		fmt.Fprintf(os.Stderr, "用法: aiknow-server hashpwd [密码]\n")
		fmt.Fprintf(os.Stderr, "       echo 'password' | aiknow-server hashpwd\n")
		os.Exit(1)
	}

	// 生成 bcrypt 哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 生成哈希失败: %v\n", err)
		os.Exit(1)
	}

	// 输出哈希值
	fmt.Println(string(hash))
}
