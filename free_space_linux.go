//go:build linux
// +build linux

package main

import (
	"fmt"
	"syscall"
)

// getLinuxFreeSpace Linux获取挂载路径可用空间（字节）
func getLinuxFreeSpace(mountPath string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(mountPath, &stat); err != nil {
		return 0, fmt.Errorf("调用statfs失败: %v", err)
	}
	// 可用空间 = 块大小 * 可用块数
	return uint64(stat.Bsize) * uint64(stat.Bavail), nil
}

// getWindowsFreeSpace Windows函数占位（避免编译错误）
func getWindowsFreeSpace(drive string) (uint64, error) {
	return 0, fmt.Errorf("当前系统为Linux，不支持Windows盘符查询")
}
