//go:build windows
// +build windows

package main

import (
	"syscall"
	"unsafe"
)

// Windows 下的具体实现（包含 NewLazyDLL）
func getUDiskInfoImpl(drive string) (string, error) {
	// 示例：调用 Windows DLL 获取磁盘信息（你原有的 NewLazyDLL 逻辑）
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpace := kernel32.NewProc("GetDiskFreeSpaceW")

	// 转换盘符为 Windows 格式（如 D: → D:\\）
	drivePath, err := syscall.UTF16PtrFromString(drive + "\\")
	if err != nil {
		return "", err
	}

	var sectorsPerCluster, bytesPerSector, freeClusters, totalClusters uint32
	_, _, err = getDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(drivePath)),
		uintptr(unsafe.Pointer(&sectorsPerCluster)),
		uintptr(unsafe.Pointer(&bytesPerSector)),
		uintptr(unsafe.Pointer(&freeClusters)),
		uintptr(unsafe.Pointer(&totalClusters)),
	)
	if err != nil && err.Error() != "The operation completed successfully." {
		return "", err
	}

	totalSize := uint64(sectorsPerCluster) * uint64(bytesPerSector) * uint64(totalClusters)
	return fmt.Sprintf("盘符 %s，总大小：%d MB", drive, totalSize/1024/1024), nil
}
