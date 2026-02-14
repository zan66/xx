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
	"sync/atomic"
	"time"

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

	// 正确接收blake2b.New512的两个返回值
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

// getDiskFreeSpace: 获取目标路径剩余空间（跨平台兼容）
func getDiskFreeSpace(path string) (int64, error) {
	// 创建临时文件测试写入，间接获取可用空间
	tempFile := filepath.Join(path, ".tmp_space_check")
	f, err := os.Create(tempFile)
	if err != nil {
		return 0, fmt.Errorf("创建临时文件失败（无法检测空间）: %v", err)
	}
	defer func() {
		os.Remove(tempFile) // 清理临时文件
		f.Close()
	}()

	// 逐步写入数据直到磁盘满（安全上限：100GB）
	maxTestSize := int64(100 * 1024 * 1024 * 1024)
	var written int64
	buf := make([]byte, 1024*1024) // 1MB缓冲区
	for written < maxTestSize {
		n, err := f.Write(buf)
		if err != nil {
			return written, nil
		}
		written += int64(n)
	}

	return maxTestSize, nil
}

// progressReader: 带进度显示的读取器（用于校验进度）
type progressReader struct {
	r        io.Reader
	total    int64
	read     atomic.Int64
	lastTime time.Time
}

func newProgressReader(r io.Reader, total int64) *progressReader {
	return &progressReader{
		r:        r,
		total:    total,
		lastTime: time.Now(),
	}
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.r.Read(p)
	pr.read.Add(int64(n))

	// 每秒更新一次进度（避免刷屏）
	now := time.Now()
	if now.Sub(pr.lastTime) >= time.Second || pr.read.Load() >= pr.total || err == io.EOF {
		pr.lastTime = now
		pr.printProgress()
	}
	return n, err
}

func (pr *progressReader) printProgress() {
	read := pr.read.Load()
	percent := float64(read) / float64(pr.total) * 100
	fmt.Printf("\r校验进度: %.2f%% | 已校验: %.2f GB / %.2f GB",
		percent,
		float64(read)/1024/1024/1024,
		float64(pr.total)/1024/1024/1024)
	if read >= pr.total || err == io.EOF {
		fmt.Println() // 进度完成后换行
	}
}

// writeWithProgress: 带进度显示的文件写入函数
func writeWithProgress(f *os.File, data []byte) error {
	total := int64(len(data))
	var written int64
	bufSize := blockSize
	lastTime := time.Now()

	for written < total {
		end := written + bufSize
		if end > total {
			end = total
		}

		n, err := f.Write(data[written:end])
		if err != nil {
			return fmt.Errorf("写入失败: %v", err)
		}

		written += int64(n)

		// 每秒更新一次进度（避免刷屏）
		now := time.Now()
		if now.Sub(lastTime) >= time.Second || written >= total {
			lastTime = now
			percent := float64(written) / float64(total) * 100
			fmt.Printf("\r写入进度: %.2f%% | 已写入: %.2f GB / %.2f GB",
				percent,
				float64(written)/1024/1024/1024,
				float64(total)/1024/1024/1024)
		}
	}
	fmt.Println() // 写入完成后换行
	return nil
}

// writeAndVerify: 写入文件并校验（带进度+哈希显示）
func writeAndVerify(round int) (string, error) {
	fmt.Printf("\n===== 第 %d 轮开始 =====\n", round+1)

	// 1. 获取U盘点剩余空间
	freeSpace, err := getDiskFreeSpace(targetPath)
	if err != nil {
		return "", fmt.Errorf("获取剩余空间失败: %v", err)
	}
	if freeSpace < blockSize {
		return "", fmt.Errorf("剩余空间不足（%d字节 < %d字节）", freeSpace, blockSize)
	}
	fmt.Printf("U盘剩余空间: %.2f GB\n", float64(freeSpace)/1024/1024/1024)

	// 2. 生成固定数据和校验和（预留100MB空间）
	actualWriteSize := freeSpace - 100*1024*1024
	if actualWriteSize < blockSize {
		return "", fmt.Errorf("预留空间后可用空间不足（%d字节 < %d字节）", actualWriteSize, blockSize)
	}
	data, checksum, err := generateFixedData(actualWriteSize)
	if err != nil {
		return "", fmt.Errorf("生成数据失败: %v", err)
	}

	// 3. 写入文件到U盘
	filePath := filepath.Join(targetPath, "udisk_fixed_data.bin")
	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %v", err)
	}
	defer f.Close()

	// 带进度写入数据
	fmt.Printf("开始写入文件: %s (总大小: %.2f GB)\n", filePath, float64(actualWriteSize)/1024/1024/1024)
	if err := writeWithProgress(f, data); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}

	// 强制刷盘（确保数据写入硬件）
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("刷盘失败: %v", err)
	}
	fmt.Println("文件写入完成，开始刷盘校验...")

	// 4. 校验文件
	verifyFile, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开校验文件失败: %v", err)
	}
	defer verifyFile.Close()

	// 带进度读取并校验
	fmt.Printf("开始校验文件: %s\n", filePath)
	verifyHash, err := blake2b.New512(nil)
	if err != nil {
		return "", fmt.Errorf("创建校验用BLAKE2b哈希器失败: %v", err)
	}
	// 包装带进度的读取器
	progressR := newProgressReader(verifyFile, actualWriteSize)
	if _, err := io.Copy(verifyHash, progressR); err != nil {
		return "", fmt.Errorf("读取校验文件失败: %v", err)
	}
	verifyChecksum := verifyHash.Sum(nil)

	// 对比校验和
	if string(verifyChecksum) != string(checksum) {
		return "", fmt.Errorf("校验失败: 写入前后校验和不一致")
	}

	// 格式化哈希值为16进制字符串（方便查看）
	checksumHex := fmt.Sprintf("%x", checksum)
	fmt.Printf("第 %d 轮校验通过！\n", round+1)
	fmt.Printf("本轮数据BLAKE2b哈希值: %s\n", checksumHex)

	// 5. 删除文件（清理U盘）
	if err := os.Remove(filePath); err != nil {
		return checksumHex, fmt.Errorf("删除文件失败: %v", err)
	}

	return checksumHex, nil
}

func main() {
	// 打印系统信息
	fmt.Printf("运行系统: %s %s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("目标路径: %s\n", targetPath)
	fmt.Printf("重复次数: %d\n", repeat)
	fmt.Println("============================")

	// 校验目标路径是否可写
	testFile := filepath.Join(targetPath, ".tmp_write_test")
	f, err := os.Create(testFile)
	if err != nil {
		log.Fatalf("目标路径不可写: %v", err)
	}
	f.Close()
	os.Remove(testFile)

	// 存储每轮的哈希值
	roundHashes := make([]string, repeat)

	// 循环执行写入+校验
	for i := 0; i < repeat; i++ {
		hashStr, err := writeAndVerify(i)
		if err != nil {
			log.Fatalf("第 %d 轮执行失败: %v", i+1, err)
		}
		roundHashes[i] = hashStr
		fmt.Printf("===== 第 %d 轮完成 =====\n", i+1)
	}

	// 汇总所有轮次的哈希值
	fmt.Println("\n============================")
	fmt.Println("所有轮次执行完成，全部校验通过！")
	fmt.Println("各轮次BLAKE2b哈希值汇总:")
	for i, hashStr := range roundHashes {
		fmt.Printf("第 %d 轮: %s\n", i+1, hashStr)
	}
}
