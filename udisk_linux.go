//go:build linux
// +build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func init() {
	// 给全局变量赋值，绑定Linux实现
	getUDiskInfoImpl = linuxUDiskInfo
}

// Linux 下的具体实现（无 NewLazyDLL）
func linuxUDiskInfo(mountPath string) (string, error) {
	// 校验挂载路径是否存在
	if !filepath.IsAbs(mountPath) {
		return "", fmt.Errorf("挂载路径必须是绝对路径（如 /mnt/udisk）")
	}
	info, err := os.Stat(mountPath)
	if err != nil {
		return "", fmt.Errorf("路径不存在：%v", err)
	}

	// 示例：获取U盘挂载路径的基本信息
	return fmt.Sprintf("挂载路径 %s，是否目录：%t，权限：%s",
		mountPath, info.IsDir(), info.Mode().String()), nil
}
