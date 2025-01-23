package main

import (
	"MarkdownServer/fastcgi"
	"MarkdownServer/goldmark"
	"MarkdownServer/goldmark/extension"
	"MarkdownServer/goldmark/parser"
	"MarkdownServer/goldmark/renderer/html"
	"bytes"
	"net/http"
	"os"
	"path"
	"strings"
)

// 全局配置
var GlobalConfig Config

// 主函数
func main() {
	GlobalConfig = GetConfig()
	FmtPrint("开源：https://github.com/zgcwkjOpenProject/GO_MarkdownServer")
	FmtPrint("作者：zgcwkj")
	FmtPrint("版本：20250123_001")
	FmtPrint("请尊重开源协议，保留作者信息！")
	http.HandleFunc("/", markDownFunc)
	server := fastcgi.NewFcgiServer(GlobalConfig.Host, nil)
	FmtPrint("Markdown Server start")
	FmtPrint("Host: " + GlobalConfig.Host)
	err := server.Start()
	if err != nil {
		FmtPrint("Markdown start error")
	}
}

// 读取解析函数
func markDownFunc(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 读取文件
	filePath := request.Header.Get("FilePath")
	data, err := os.ReadFile(filePath)
	if err != nil {
		//log.Fatal(err)
		writer.Write([]byte("<title>File not found</title>"))
		writer.Write([]byte(GlobalConfig.Error))
		return
	}
	var buf bytes.Buffer
	// 插入开始文本
	start := GlobalConfig.InsertStart
	if len(start) > 0 {
		//是路径时，读取文件
		if strings.Contains(start, ".txt") {
			startData, _ := os.ReadFile(start)
			buf.Write(startData)
		} else {
			buf.Write([]byte(start))
		}
	}
	// 插入网站标题
	webTitle := GlobalConfig.WebTitle
	if len(webTitle) > 0 {
		if strings.Contains(webTitle, "{fileName}") {
			fileName := strings.TrimSuffix(path.Base(filePath), path.Ext(filePath))
			webTitle = strings.Replace(webTitle, "{fileName}", fileName, -1)
		}
		buf.Write([]byte("<title>" + webTitle + "</title>"))
	}
	// 解析文件
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
	md.Convert(data, &buf)
	// 插入结束文本
	end := GlobalConfig.InsertEnd
	if len(end) > 0 {
		//是路径时，读取文件
		if strings.Contains(end, ".txt") {
			endData, _ := os.ReadFile(end)
			buf.Write(endData)
		} else {
			buf.Write([]byte(end))
		}
	}
	// 写入响应
	writer.Write(buf.Bytes())
}
