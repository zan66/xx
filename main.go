package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// 预定义固定的基准数据块（100MB），保证每次写入内容一致
var fixedDataBlock []byte

// 初始化固定数据块（只执行一次）
func initFixedDataBlock() error {
	blockSize := 100 * 1024 * 1024 // 100MB
	fixedDataBlock = make([]byte, blockSize)
	// 固定生成规则：循环0-255，确保内容可复现
	for i := 0; i < blockSize; i++ {
		fixedDataBlock[i] = byte(i % 256)
	}
	return nil
}

// 获取指定盘符的可用空间（字节）
// 修复点1：将原本的 _ 替换为实际变量 totalFreeBytes（解决 _ 不能作为值的错误）
func getDiskFreeSpace(drive string) (uint64, uint64, error) {
	// 转换为Windows API需要的UTF16格式
	driveUTF16 := utf16.Encode([]rune(drive))
	driveUTF16 = append(driveUTF16, 0) // 以空字符结尾

	var freeBytes, totalBytes, totalFreeBytes uint64 // 新增实际变量接收第三个返回值
	// 调用Windows API GetDiskFreeSpaceExW
	ret, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW").Call(
		uintptr(unsafe.Pointer(&driveUTF16[0])),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)), // 替换原本的 _，使用实际变量
	)
	if ret == 0 {
		return 0, 0, fmt.Errorf("获取磁盘空间失败: %v", err)
	}

	// 打印磁盘信息
	fmt.Printf("\n=== 磁盘信息 ===\n")
	fmt.Printf("盘符：%s\n", drive)
	fmt.Printf("总容量：%.2f GB\n", float64(totalBytes)/(1024*1024*1024))
	fmt.Printf("可用空间：%.2f GB\n", float64(freeBytes)/(1024*1024*1024))

	return freeBytes, totalBytes, nil
}

// 写入固定内容的文件占满U盘（保证SHA1固定）
func writeFixedFile(drive, fileName string, freeSpace uint64) (string, error) {
	filePath := filepath.Join(drive, fileName)
	blockSize := uint64(len(fixedDataBlock))

	// 创建文件（覆盖已有文件）
	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %v", err)
	}
	defer f.Close()

	remaining := freeSpace
	fmt.Printf("\n开始写入文件：%s\n", filePath)
	startTime := time.Now()

	for remaining > 0 {
		writeSize := blockSize
		if remaining < blockSize {
			writeSize = remaining
		}

		// 写入固定数据块的前writeSize字节
		_, err := f.Write(fixedDataBlock[:writeSize])
		if err != nil {
			os.Remove(filePath) // 清理未完成文件
			return "", fmt.Errorf("写入失败: %v", err)
		}

		remaining -= writeSize

		// 实时打印进度
		progress := float64(freeSpace-remaining)/float64(freeSpace)*100
		fmt.Printf("写入进度：%.1f%%\r", progress)
	}

	// 强制刷盘，确保内容完全写入
	if err := f.Sync(); err != nil {
		os.Remove(filePath)
		return "", fmt.Errorf("同步文件失败: %v", err)
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\n写入完成，耗时：%.2f 秒\n", elapsed.Seconds())
	return filePath, nil
}

// 计算文件的SHA1哈希值
func calculateSHA1(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	hash := sha1.New()
	buf := make([]byte, 64*1024) // 64KB缓冲区，避免内存溢出
	fmt.Printf("计算SHA1值中...\n")
	startTime := time.Now()

	for {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("读取文件失败: %v", err)
		}
		if n == 0 {
			break
		}
		hash.Write(buf[:n])
	}

	sha1Str := hex.EncodeToString(hash.Sum(nil))
	elapsed := time.Since(startTime)
	fmt.Printf("SHA1计算完成：%s（耗时：%.2f 秒）\n", sha1Str, elapsed.Seconds())

	return sha1Str, nil
}

// 删除指定文件
func deleteFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", filePath)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("删除失败: %v", err)
	}
	fmt.Printf("文件已删除：%s\n", filePath)
	return nil
}

// 格式化盘符输入（兼容 E: / E / E:\ 等格式）
func formatDrive(drive string) string {
	if len(drive) == 1 && drive[0] >= 'A' && drive[0] <= 'Z' {
		return drive + `:\`
	} else if len(drive) == 2 && drive[1] == ':' {
		return drive + `\`
	}
	return drive
}

func main() {
	// 初始化固定数据块
	if err := initFixedDataBlock(); err != nil {
		fmt.Printf("初始化数据块失败：%v\n", err)
		return
	}

	// 1. 输入U盘盘符
	var drive string
	fmt.Print("请输入U盘盘符（如 E:\\）：")
	fmt.Scanln(&drive)
	drive = formatDrive(drive)

	// 验证盘符是否存在
	if _, err := os.Stat(drive); os.IsNotExist(err) {
		fmt.Printf("错误：盘符 %s 不存在！\n", drive)
		return
	}

	// 2. 获取U盘空间信息
	freeSpace, totalSpace, err := getDiskFreeSpace(drive)
	if err != nil {
		fmt.Printf("获取磁盘空间失败：%v\n", err)
		return
	}

	// 修复点2：使用 totalSpace 变量（解决声明未使用的错误）
	fmt.Printf("U盘总容量：%.2f GB，可用容量：%.2f GB\n", 
		float64(totalSpace)/(1024*1024*1024), 
		float64(freeSpace)/(1024*1024*1024))

	// 预留1MB空间，避免完全占满导致系统异常
	const reserveSpace = 1024 * 1024 // 1MB
	if freeSpace < reserveSpace {
		fmt.Println("错误：U盘可用空间不足（至少需要1MB）！")
		return
	}
	freeSpace -= reserveSpace

	// 3. 重复5次写入-校验-删除流程
	fileName := "usb_test_file.tmp"
	var baselineSHA1 string
	results := make([]struct {
		round        int
		sha1         string
		isConsistent bool
	}, 0)

	fmt.Printf("\n=== 开始5次重复验证 ===\n")
	for round := 1; round <= 5; round++ {
		fmt.Printf("\n==================== 第 %d 次验证 ====================\n", round)

		// 写入文件
		filePath, err := writeFixedFile(drive, fileName, freeSpace)
		if err != nil {
			fmt.Printf("第 %d 次写入失败：%v，终止验证！\n", round, err)
			break
		}

		// 计算SHA1
		sha1Str, err := calculateSHA1(filePath)
		if err != nil {
			deleteFile(filePath)
			fmt.Printf("第 %d 次校验失败：%v，终止验证！\n", round, err)
			break
		}

		// 记录基准值（第一次）
		if round == 1 {
			baselineSHA1 = sha1Str
			fmt.Printf("✅ 第一次SHA1基准值：%s\n", baselineSHA1)
		} else {
			isConsistent := sha1Str == baselineSHA1
			results = append(results, struct {
				round        int
				sha1         string
				isConsistent bool
			}{round, sha1Str, isConsistent})
			status := "✅ 一致"
			if !isConsistent {
				status = "❌ 不一致"
			}
			fmt.Printf("%s 第 %d 次SHA1值：%s\n", status, round, sha1Str)
		}

		// 删除文件
		if err := deleteFile(filePath); err != nil {
			fmt.Printf("⚠️ 第 %d 次删除文件失败：%v，请手动清理！\n", round, err)
		}
	}

	// 4. 输出最终报告
	fmt.Printf("\n=== 最终验证报告 ===\n")
	if baselineSHA1 == "" {
		fmt.Println("❌ 未完成完整验证流程")
		return
	}

	fmt.Printf("基准SHA1值：%s\n", baselineSHA1)
	allConsistent := true
	for _, res := range results {
		status := "✅ 一致"
		if !res.isConsistent {
			status = "❌ 不一致"
			allConsistent = false
		}
		fmt.Printf("第 %d 次：%s %s\n", res.round, res.sha1, status)
	}

	if allConsistent && len(results) == 4 {
		fmt.Println("\n🎉 所有5次验证SHA1值完全一致！U盘数据写入稳定性验证通过！")
	} else {
		fmt.Println("\n⚠️ 部分验证SHA1值不一致，U盘可能存在稳定性问题！")
	}
}
