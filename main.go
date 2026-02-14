package main

import (
	"fmt"
	"os"
)

// 声明 getUDiskInfoImpl 为全局函数（告诉编译器：这个函数由各系统文件实现）
var getUDiskInfoImpl func(string) (string, error)

// 统一的U盘信息获取入口
func GetUDiskInfo(path string) (string, error) {
	if getUDiskInfoImpl == nil {
		return "", fmt.Errorf("当前系统不支持（仅支持 Linux/Windows）")
	}
	return getUDiskInfoImpl(path)
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
	fmt.Printf("U盘信息：%s\n", info)
}
