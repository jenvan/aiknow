// Package cli 放各子命令共用的小工具。
package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// Prog 返回当前可执行文件名（去掉目录与 .exe 后缀），用于帮助文本。
//
// 帮助里写死 "aiknow" 会在重命名二进制后立刻过时（比如历史上叫 aiknow-server），
// 所以一律从 os.Args[0] 推导。
func Prog() string {
	base := filepath.Base(os.Args[0])
	return strings.TrimSuffix(base, ".exe")
}
