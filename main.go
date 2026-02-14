package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/blake2b"
)

// 声明 getUDiskInfoImpl 为全局函数
var getUDiskInfoImpl func(string) (string, error)

// 通用的 Blake2b 哈希计算函数（替换原 SHA1 逻辑）
func CalculateBlake2bHash(data []byte) []byte {
	// 创建 256 位的 Blake2b 哈希器（等价于 SHA256，比 SHA1 更安全）
	hash, err := blake2b.New256(nil)
	if err != nil {
		panic(fmt.Sprintf("创建 Blake2b 哈希器失败：%v", err))
	}
	hash.Write(data)
	return hash.Sum(nil)
}

// 统一的U盘信息获取入口
func GetUDiskInfo(path string) (string, error) {
	if getUDiskInfoImpl == nil {
		return "", fmt.Errorf("当前系统不支持（仅支持 Linux/Windows）")
	}
	// 获取U盘信息后计算 Blake2b 哈希
	info, err := getUDiskInfoImpl(path)
	if err != nil {
		return "", err
	}
	// 对U盘信息计算 Blake2b 哈希（替换原 SHA1 计算）
	hash := CalculateBlake2bHash([]byte(info))
	return fmt.Sprintf("%s\nBlake2b 哈希：%x", info, hash), nil
}

func main() {
	// 判断当前系统
	var inputTip string
	if os.PathSeparator == '\\' { // Windows
		inputTip = "请输入U盘盘符（如 D:）："
	} else { // Linux
		inputTip = "请输入U盘挂载路径（如 /mnt/udisk）："
	}

	// 交互式输入
	var inputPath string
	fmt.Print(inputTip)
	fmt.Scanln(&inputPath)

	// 调用系统专属逻辑
	info, err := GetUDiskInfo(inputPath)
	if err != nil {
		fmt.Printf("获取U盘信息失败：%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("U盘信息：\n%s\n", info)
}
