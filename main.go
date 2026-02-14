package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/crypto/blake2b"
)

// 全局变量定义
var (
	targetPath string // Windows: 盘符（如 D:\）；Linux: 挂载路径（如 /mnt/udisk）
	repeat     int    // 重复次数，默认5次
	blockSize  int64  // 写入块大小，默认4MB
)

func init() {
	// 解析命令行参数
	flag.StringVar(&targetPath, "path", "", "Windows: U盘盘符（如 D:\\）；Linux: 挂载路径（如 /mnt/udisk）")
	flag.IntVar(&repeat, "repeat", 5, "写入+校验重复次数（默认5次）")
	flag.Int64Var(&blockSize, "block", 4*1024*1024, "写入块大小（默认4MB）")
	flag.Parse()

	// 校验参数
	if targetPath == "" {
		log.Fatal("必须指定 -path 参数：Windows 传入盘符（如 D:\\），Linux 传入挂载路径（如 /mnt/udisk）")
	}

	// 标准化路径（处理不同系统路径分隔符）
	targetPath = filepath.Clean(targetPath)
}

// generateFixedData: 生成固定内容的字节流（基于BLAKE2b种子）
func generateFixedData(size int64) ([]byte, []byte, error) {
	// 生成随机种子（确保每次生成的固定数据一致）
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, nil, fmt.Errorf("生成种子失败: %v", err)
	}

	// 创建BLAKE2b哈希器
	h, err := blake2b.New512(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("创建BLAKE2b哈希器失败: %v", err)
	}

	// 生成固定数据
	data := make([]byte, size)
	if _, err := io.ReadFull(&fixedReader{h: h, seed: seed}, data); err != nil {
		return nil, nil, fmt.Errorf("生成固定数据失败: %v", err)
	}

	// 计算数据的BLAKE2b校验和
	checksum := blake2b.Sum512(data)
	return data, checksum[:], nil
}

// fixedReader: 基于BLAKE2b的固定数据读取器
type fixedReader struct {
	h    hash.Hash
	seed []byte
}

func (f *fixedReader) Read(p []byte) (n int, err error) {
	// 重置哈希器并写入种子
	f.h.Reset()
	f.h.Write(f.seed)

	// 生成哈希值填充输出
	hashBytes := f.h.Sum(nil)
	n = copy(p, hashBytes)
	if n < len(p) {
		// 循环填充直到满
		for i := n; i < len(p); i += len(hashBytes) {
			copy(p[i:], hashBytes[:len(p)-i])
		}
		n = len(p)
	}
	return n, nil
}

// getDiskFreeSpace: 获取目标路径剩余空间（跨平台）
func getDiskFreeSpace(path string) (int64, error) {
	var stat os.FileInfo
	var err error

	// 不同系统获取磁盘信息
	if runtime.GOOS == "windows" {
		// Windows: 直接获取盘符的剩余空间
		stat, err = os.Stat(path)
	} else {
		// Linux: 获取挂载路径的剩余空间
		stat, err = os.Stat(path)
	}
	if err != nil {
		return 0, fmt.Errorf("获取路径信息失败: %v", err)
	}

	// 获取文件系统信息
	fsInfo, ok := stat.Sys().(*syscall.Statfs_t)
	if !ok {
		return 0, fmt.Errorf("不支持的文件系统类型")
	}

	// 计算剩余空间 = 块大小 * 可用块数
	freeSpace := int64(fsInfo.Bsize) * int64(fsInfo.Bavail)
	return freeSpace, nil
}

// writeAndVerify: 写入文件并校验
func writeAndVerify(round int) error {
	fmt.Printf("===== 第 %d 轮开始 =====\n", round+1)

	// 1. 获取U盘点剩余空间
	freeSpace, err := getDiskFreeSpace(targetPath)
	if err != nil {
		return fmt.Errorf("获取剩余空间失败: %v", err)
	}
	if freeSpace < blockSize {
		return fmt.Errorf("剩余空间不足（%d字节 < %d字节）", freeSpace, blockSize)
	}
	fmt.Printf("U盘剩余空间: %d MB\n", freeSpace/1024/1024)

	// 2. 生成固定数据和校验和
	data, checksum, err := generateFixedData(freeSpace)
	if err != nil {
		return fmt.Errorf("生成数据失败: %v", err)
	}

	// 3. 写入文件到U盘
	filePath := filepath.Join(targetPath, "udisk_fixed_data.bin")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer f.Close()

	// 写入数据
	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}
	// 强制刷盘（确保数据写入硬件）
	if err := f.Sync(); err != nil {
		return fmt.Errorf("刷盘失败: %v", err)
	}
	fmt.Printf("文件写入完成: %s\n", filePath)

	// 4. 校验文件
	verifyFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开校验文件失败: %v", err)
	}
	defer verifyFile.Close()

	// 读取文件并计算校验和
	verifyHash := blake2b.New512(nil)
	if _, err := io.Copy(verifyHash, verifyFile); err != nil {
		return fmt.Errorf("读取校验文件失败: %v", err)
	}
	verifyChecksum := verifyHash.Sum(nil)

	// 对比校验和
	if string(verifyChecksum) != string(checksum) {
		return fmt.Errorf("校验失败: 写入前后校验和不一致")
	}
	fmt.Printf("第 %d 轮校验通过\n", round+1)

	// 5. 删除文件（可选，根据需求保留）
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("删除文件失败: %v", err)
	}

	return nil
}

func main() {
	// 打印系统信息
	fmt.Printf("运行系统: %s %s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("目标路径: %s\n", targetPath)
	fmt.Printf("重复次数: %d\n", repeat)

	// 循环执行写入+校验
	for i := 0; i < repeat; i++ {
		if err := writeAndVerify(i); err != nil {
			log.Fatalf("第 %d 轮执行失败: %v", i+1, err)
		}
		fmt.Printf("===== 第 %d 轮完成 =====\n\n", i+1)
	}

	fmt.Println("所有轮次执行完成，全部校验通过！")
}
