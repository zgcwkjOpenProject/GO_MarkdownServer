# 下载

go get gitee.com/xqhero/fastcgi

# 使用

```
package main

import (
	"fmt"
	"gitee.com/xqhero/fastcgi"
	"io/ioutil"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		bytes, _ := ioutil.ReadFile("./index.html")
		writer.Write(bytes)
	})
	server := fastcgi.NewFcgiServer("127.0.0.1:9000", nil)
	err := server.Start()
	if err != nil {
		fmt.Println("fcgiserver start error")
	}
}


```

# 文档说明

https://wiki.xqhero.com/docs/tcp-ip