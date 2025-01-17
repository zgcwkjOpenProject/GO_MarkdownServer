package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// 配置文件
type Config struct {
	Host       string `json:"host"`
	InsertText struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"insertText"`
}

// 获取配置
func GetConfig() Config {
	var config Config
	filePath := "config.json"
	data, err := os.ReadFile(filePath)
	if err != nil {
		FmtPrint("配置文件不存在", err)
		os.Exit(0)
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		FmtPrint("读取配置文件出错", err)
		os.Exit(0)
	}
	return config
}

// 定义内置的打印语句
func FmtPrint(data ...any) {
	date := time.Now().Format("2006-01-02 15:04:05")
	if len(data) == 1 {
		fmt.Println(date+": ", data[0])
	} else {
		fmt.Println(date+": ", data)
	}
}
