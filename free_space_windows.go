//go:build windows
// +build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// getWindowsFreeSpace Windows获取磁盘可用空间（字节）
func getWindowsFreeSpace(drive string) (uint64, error) {
	kernel32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		return 0, fmt.Errorf("加载kernel32.dll失败: %v", err)
	}
	defer syscall.FreeLibrary(kernel32)

	getDiskFreeSpaceEx, err := syscall.GetProcAddress(kernel32, "GetDiskFreeSpaceExW")
	if err != nil {
		return 0, fmt.Errorf("获取GetDiskFreeSpaceExW函数地址失败: %v", err)
	}

	var freeBytesAvailable uint64
	drivePtr, err := syscall.UTF16PtrFromString(drive)
	if err != nil {
		return 0, fmt.Errorf("转换盘符路径失败: %v", err)
	}

	ret, _, err := syscall.Syscall6(
		uintptr(getDiskFreeSpaceEx),
		4,
		uintptr(unsafe.Pointer(drivePtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		0, 0, 0, 0,
	)
	if ret == 0 {
		return 0, fmt.Errorf("调用GetDiskFreeSpaceExW失败: %v", err)
	}
	return freeBytesAvailable, nil
}

// getLinuxFreeSpace Linux函数占位（避免编译错误）
func getLinuxFreeSpace(mountPath string) (uint64, error) {
	return 0, fmt.Errorf("当前系统为Windows，不支持Linux路径查询")
}
