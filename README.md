# MarkdownServer

## 功能

实现 Nginx 直接解析 md 文件，并返回到页面

## 配置

> 配置文件

```
host => 监听服务地址
insertText => 用于插入样式等字符
	start => 开始文本
	end => 结束文本
```

> 配置 Nginx

```
#Markdown
location ~ \.md(.*)$ {
	fastcgi_pass   127.0.0.1:9900;
	fastcgi_index index.md;
	fastcgi_split_path_info  ^((?U).+\.md)(/?.+)$;
	fastcgi_param  SCRIPT_FILENAME  $document_root$fastcgi_script_name;
	fastcgi_param  PATH_INFO  $fastcgi_path_info;
	fastcgi_param  PATH_TRANSLATED  $document_root$fastcgi_path_info;
	include        fastcgi_params;
}
```

## 感谢

> 使用到开源

- gitee.com/xqhero/fastcgi
- github.com/yuin/goldmark
