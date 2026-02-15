package main

import (
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/crypto/blake2b"
)

// 分块大小（64MB，可根据内存调整）
const chunkSize = 64 * 1024 * 1024

// 生成固定的BLAKE2b哈希数据块
func generateFixedBlock(h hash.Hash, blockSize int) ([]byte, error) {
	// 生成固定种子（保证数据固定）
	seed := make([]byte, 32)
	copy(seed, []byte("usb-hash-check-fixed-seed-2026"))

	// 用BLAKE2b基于种子生成固定数据块
	h.Reset()
	h.Write(seed)
	seedHash := h.Sum(nil)

	block := make([]byte, blockSize)
	pos := 0
	for pos < blockSize {
		h.Reset()
		h.Write(seedHash)
		chunk := h.Sum(nil)
		copy(block[pos:], chunk)
		pos += len(chunk)
		if pos+len(chunk) > blockSize {
			break
		}
	}
	return block, nil
}

// 获取U盘可用空间（字节）- 通用入口
func getUsbFreeSpace(path string) (uint64, error) {
	if runtime.GOOS == "windows" {
		// Windows: 盘符格式如 "D:"
		return getWindowsFreeSpace(path)
	} else {
		// Linux: 挂载路径如 "/mnt/usb"
		return getLinuxFreeSpace(path)
	}
}

// 分块写入文件到U盘
func writeFileToUsb(usbPath, fileName string, totalSize uint64, h hash.Hash) error {
	filePath := filepath.Join(usbPath, fileName)
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	    if err := f.Sync(); err != nil {
        return fmt.Errorf("刷新文件缓存失败: %v", err)
    }
	defer f.Close()

	// 生成固定数据块
	fixedBlock, err := generateFixedBlock(h, chunkSize)
	if err != nil {
		return fmt.Errorf("生成固定块失败: %v", err)
	}

	var written uint64
	var progress float64
	for written < totalSize {
		// 计算本次写入大小（避免最后一块超出总大小）
		writeSize := chunkSize
		remaining := totalSize - written
		if uint64(writeSize) > remaining {
			writeSize = int(remaining)
		}

		// 写入数据
		n, err := f.Write(fixedBlock[:writeSize])
		if err != nil {
			return fmt.Errorf("写入失败: %v", err)
		}
		written += uint64(n)

		// 计算并显示进度
		progress = float64(written) / float64(totalSize) * 100
		fmt.Printf("\r写入进度: %.2f%%", progress)
	}
	fmt.Println() // 换行
	return nil
}

// 分块校验文件哈希
func verifyFileHash(usbPath, fileName string, totalSize uint64, h hash.Hash) ([]byte, error) {
	filePath := filepath.Join(usbPath, fileName)
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %v", err)
	}
	defer f.Close()

	h.Reset()
	var read uint64
	var progress float64
	buf := make([]byte, chunkSize)

	for read < totalSize {
		// 计算本次读取大小
		readSize := chunkSize
		remaining := totalSize - read
		if uint64(readSize) > remaining {
			readSize = int(remaining)
		}

		// 读取数据并更新哈希
		n, err := f.Read(buf[:readSize])
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("读取失败: %v", err)
		}
		if n == 0 {
			break
		}
		h.Write(buf[:n])
		read += uint64(n)

		// 显示校验进度
		progress = float64(read) / float64(totalSize) * 100
		fmt.Printf("\r校验进度: %.2f%%", progress)
	}
	fmt.Println() // 换行
	return h.Sum(nil), nil
}

// 单次写入-校验流程（含1MB预留空间+自动删除文件）
func runSingleTest(usbPath string, testNum int) error {
	fmt.Printf("\n=== 开始第 %d 次测试 ===\n", testNum)
	fileName := fmt.Sprintf("usb_test_%d.dat", testNum)
	filePath := filepath.Join(usbPath, fileName) // 提前定义文件路径，方便后续删除

	// 1. 获取U盘可用空间
	freeSpace, err := getUsbFreeSpace(usbPath)
	if err != nil {
		return fmt.Errorf("获取可用空间失败: %v", err)
	}
	if freeSpace == 0 {
		return fmt.Errorf("U盘无可用空间")
	}

	// 核心修改：预留1MB空余空间，避免写满触发系统错误
	const reserveSpace = 1 * 1024 * 1024 // 1MB = 1024*1024字节
	var writeSize uint64
	if freeSpace > reserveSpace {
		writeSize = freeSpace - reserveSpace
	} else {
		return fmt.Errorf("U盘可用空间不足（小于1MB），无法测试")
	}

	// 打印可用空间和实际写入空间（方便查看）
	fmt.Printf("U盘可用空间: %.2f GB\n", float64(freeSpace)/(1024*1024*1024))
	fmt.Printf("实际写入空间: %.2f GB（预留1MB空余空间）\n", float64(writeSize)/(1024*1024*1024))

	// 2. 初始化BLAKE2b哈希器
	h, err := blake2b.New256(nil)
	if err != nil {
		return fmt.Errorf("初始化BLAKE2b失败: %v", err)
	}

	// 3. 分块写入文件（修改：用writeSize替代freeSpace）
	fmt.Println("开始写入文件...")
	if err := writeFileToUsb(usbPath, fileName, writeSize, h); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}
	
	// 核心优化2：写入完成后延迟2秒，模拟U盘静置场景，测试稳定性
	fmt.Println("写入完成，等待2秒后开始校验...")
	import "time" // 若文件顶部未导入time，需先添加
	time.Sleep(2 * time.Second)


	// 4. 校验文件哈希（修改：用writeSize替代freeSpace）
	fmt.Println("开始校验文件哈希...")
	hashBytes, err := verifyFileHash(usbPath, fileName, writeSize, h)
	if err != nil {
		// 校验失败时也尝试删除文件，避免残留
		fmt.Printf("校验失败，尝试删除残留文件: %s\n", filePath)
		_ = os.Remove(filePath)
		return fmt.Errorf("校验哈希失败: %v", err)
	}
	fmt.Printf("第 %d 次校验完成，BLAKE2b哈希值: %x\n", testNum, hashBytes)

	// 5. 校验完成后强制删除文件，释放空间
	fmt.Printf("开始删除测试文件: %s\n", filePath)
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("删除文件失败: %v", err)
	}
	fmt.Printf("第 %d 次测试文件已删除，释放U盘空间\n", testNum)

	return nil
}

func main() {
	// 解析命令行参数
	usbPath := flag.String("path", "", "U盘路径（Windows: 盘符如D:, Linux: 挂载路径如/mnt/usb）")
	flag.Parse()

	if *usbPath == "" {
		fmt.Println("请指定U盘路径：")
		fmt.Println("  Windows: ./usb-hash-check -path D:")
		fmt.Println("  Linux: ./usb-hash-check -path /mnt/usb")
		os.Exit(1)
	}

	// 重复5次测试
	for i := 1; i <= 5; i++ {
		if err := runSingleTest(*usbPath, i); err != nil {
			fmt.Printf("第 %d 次测试失败: %v\n", i, err)
			os.Exit(1)
		}
	}

	fmt.Println("\n=== 所有测试完成 ===")
}
