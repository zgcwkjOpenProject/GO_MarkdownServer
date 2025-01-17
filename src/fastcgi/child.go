package fastcgi

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 请求结构体
type Request struct {
	pipeWriter *io.PipeWriter
	reqId      uint16
	params     map[string]string
	buf        [1024]byte
	rawParams  []byte
	keepAlive  bool
}

var emptyBody = io.NopCloser(strings.NewReader(""))

var ErrConnClosed = errors.New("fcgi: connection to web server closed")

// 构造新请求
func NewRequest(reqId uint16, flags uint8) *Request {
	r := &Request{
		reqId:     reqId,
		params:    map[string]string{},
		keepAlive: flags&flagKeepAlive != 0,
	}
	r.rawParams = r.buf[:0]
	return r
}

// 解析key-value
// 0x220x33namea
// 0x220x33namea
func (req *Request) ParseParams() {
	text := req.rawParams
	req.rawParams = nil
	for len(text) > 0 {
		// 得到key长度
		keylen, n := readSize(text)
		if n == 0 {
			fmt.Println("n===0")
			return
		}
		text = text[n:]
		// 得到值长度
		valuelen, n := readSize(text)
		if n == 0 {
			return
		}
		text = text[n:]
		if keylen+valuelen > uint32(len(text)) {
			return
		}
		key := string(text[:keylen])
		text = text[keylen:]
		value := string(text[:valuelen])
		text = text[valuelen:]
		// 加入key-value pair
		req.params[key] = value
	}
}

// 实现 ResponseWriter 接口
type Response struct {
	req            *Request
	header         http.Header
	code           int
	wroteHeader    bool
	wroteCGIHeader bool
	w              *bufWriter
}

func NewResponse(c *ConnChild, req *Request) *Response {
	res := &Response{
		req:    req,
		header: http.Header{},
		w:      newWriter(c.conn, FCGI_STDOUT, req.reqId),
	}
	return res
}

func (resp *Response) Header() http.Header {
	return resp.header
}

func (resp *Response) Write(p []byte) (int, error) {
	if !resp.wroteHeader {
		resp.WriteHeader(http.StatusOK)
	}
	if !resp.wroteCGIHeader {
		//
		resp.writeCGIHeader(p)
	}
	return resp.w.Write(p)
}

func (resp *Response) WriteHeader(statusCode int) {
	if resp.wroteHeader {
		return
	}

	resp.wroteHeader = true
	resp.code = statusCode

	if statusCode == http.StatusNotModified {
		// Must not have body.
		resp.header.Del("Content-Type")
		resp.header.Del("Content-Length")
		resp.header.Del("Transfer-Encoding")
	}

	if resp.header.Get("Date") == "" {
		resp.header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}
}

func (r *Response) writeCGIHeader(p []byte) {
	if r.wroteCGIHeader {
		return
	}
	r.wroteCGIHeader = true
	fmt.Fprintf(r.w, "Status: %d %s\r\n", r.code, http.StatusText(r.code))
	if _, hasType := r.header["Content-Type"]; r.code != http.StatusNotModified && !hasType {
		r.header.Set("Content-Type", http.DetectContentType(p))
	}
	r.header.Write(r.w)
	r.w.WriteString("\r\n")
	r.w.Flush()
}

func (r *Response) Flush() {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	r.w.Flush()
}

func (r *Response) Close() error {
	r.Flush()
	return r.w.Close()
}

// connChild
type ConnChild struct {
	conn    *Conn
	handler http.Handler

	mutex    sync.RWMutex
	requests map[uint16]*Request
}

func NewConnChild(conn io.ReadWriteCloser, handler http.Handler) *ConnChild {
	c := &ConnChild{
		conn:     newConn(conn),
		handler:  handler,
		requests: make(map[uint16]*Request),
	}
	return c
}

func (srv *FcgiServer) Start() error {
	listener, err := net.Listen("tcp", srv.addr)
	if err != nil {
		return err
	}
	srv.listener = listener
	if srv.handler == nil {
		srv.handler = http.DefaultServeMux
	}
	for {
		conn, err := srv.listener.Accept()
		if err != nil {
			return err
		}
		c := NewConnChild(conn, srv.handler)
		go c.Serve()
	}
	return nil
}

func (c *ConnChild) Serve() {
	// 退出时关闭连接
	defer c.conn.Close()
	// 清理工作
	defer c.cleanUp()
	var rec Record
	for {
		if err := rec.read(c.conn.rwc); err != nil {
			//fmt.Println("读取失败")
			return
		}
		if err := c.handleRecord(&rec); err != nil {
			//fmt.Println("操作失败")
			return
		}
	}
}

func (c *ConnChild) cleanUp() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, req := range c.requests {
		if req.pipeWriter != nil {
			req.pipeWriter.CloseWithError(ErrConnClosed)
		}
	}
}

func (c *ConnChild) handleRecord(record *Record) error {
	// 读取是否有对应requestId的包
	c.mutex.RLock()
	request, ok := c.requests[record.h.RequestId]
	c.mutex.RUnlock()
	// 不存在且类型不为FCGI_BEGIN_REQUEST且类型不为FCGI_GET_VALUES 则不处理
	if !ok && record.h.Ftype != FCGI_BEGIN_REQUEST && record.h.Ftype != FCGI_GET_VALUES {
		return nil
	}
	switch record.h.Ftype {
	case FCGI_BEGIN_REQUEST:
		// 如果使用同样的序号则返回错误
		if request != nil {
			return errors.New("fcgi: received ID that is already in-flight")
		}

		var br FCGI_BeginRequestBody
		// 将内容赋值给br
		err := br.read(record.content())
		if err != nil {
			return err
		}
		// 只处理响应类型
		if br.Role != ROLE_FCGI_RESPONSE {
			// 直接返回结束
			c.conn.withEndRequest(record.h.RequestId, 0, FCGI_UNKNOW_ROLE)
			return nil
		}
		// 创建新的请求
		request = NewRequest(record.h.RequestId, br.Flags)
		c.mutex.Lock()
		c.requests[record.h.RequestId] = request
		c.mutex.Unlock()
	case FCGI_PARAMS:
		// 等所有的params都读取完毕才进行解析
		if len(record.content()) > 0 {
			request.rawParams = append(request.rawParams, record.content()...)
			return nil
		}
		// 将所有的key-value pair 进行解析
		request.ParseParams()

	case FCGI_STDIN:
		content := record.content()
		// 将其进行转换 并直接处理成http响应
		if request.pipeWriter == nil {
			var pipeReader io.ReadCloser
			if len(content) > 0 {
				pipeReader, request.pipeWriter = io.Pipe()
				fmt.Println(pipeReader)
			} else {
				pipeReader = emptyBody
			}
			// 开启线程处理请求 借助pipe的特性
			go c.serveRequest(request, pipeReader)
		}
		// 如果内容长度大于0 则将内容写入到pipe
		if len(content) > 0 {
			request.pipeWriter.Write(content)
		} else if request.pipeWriter != nil {
			// 如果收到空的stdin包 则关闭pipe写端
			request.pipeWriter.Close()
		}
	default:
		b := make([]byte, 8)
		b[0] = byte(record.h.Ftype)
		c.conn.writeRecord(FCGI_UNKNOWN_TYPE, 0, b)
	}
	return nil
}

func (c *ConnChild) serveRequest(req *Request, pr io.ReadCloser) {
	// 将fastcgi交给http处理
	resp := NewResponse(c, req)
	httpReq, err := RequestFromMap(req.params)
	if err != nil {
		resp.WriteHeader(http.StatusInternalServerError)
		c.conn.writeRecord(FCGI_STDERR, req.reqId, []byte(err.Error()))
	} else {
		httpReq.Body = pr
		c.handler.ServeHTTP(resp, httpReq)
	}
	// 将处理的结果返回给fastcgi客户端
	resp.Write([]byte(""))
	resp.Close()
	c.mutex.Lock()
	delete(c.requests, req.reqId)
	c.mutex.Unlock()
	c.conn.withEndRequest(req.reqId, 0, FCGI_REQUEST_COMPLETE)

	// 没有读取pr 则将pr内容进行清除
	// io.CopyN(ioutil.Discard, pr, 100<<20)
	pr.Close()
	if !req.keepAlive {
		c.conn.Close()
	}
}

func RequestFromMap(params map[string]string) (*http.Request, error) {
	r := new(http.Request)
	r.Method = params["REQUEST_METHOD"]
	if r.Method == "" {
		return nil, errors.New("cgi: no REQUEST_METHOD in environment")
	}

	r.Proto = params["SERVER_PROTOCOL"]
	var ok bool
	r.ProtoMajor, r.ProtoMinor, ok = http.ParseHTTPVersion(r.Proto)
	if !ok {
		return nil, errors.New("cgi: invalid SERVER_PROTOCOL version")
	}

	r.Close = true
	r.Trailer = http.Header{}
	r.Header = http.Header{}

	r.Host = params["HTTP_HOST"]

	if lenstr := params["CONTENT_LENGTH"]; lenstr != "" {
		clen, err := strconv.ParseInt(lenstr, 10, 64)
		if err != nil {
			return nil, errors.New("cgi: bad CONTENT_LENGTH in environment: " + lenstr)
		}
		r.ContentLength = clen
	}

	if ct := params["CONTENT_TYPE"]; ct != "" {
		r.Header.Set("Content-Type", ct)
	}

	// Copy "HTTP_FOO_BAR" variables to "Foo-Bar" Headers
	for k, v := range params {
		if !strings.HasPrefix(k, "HTTP_") || k == "HTTP_HOST" {
			continue
		}
		r.Header.Add(strings.ReplaceAll(k[5:], "_", "-"), v)
	}

	uriStr := params["REQUEST_URI"]
	if uriStr == "" {
		// Fallback to SCRIPT_NAME, PATH_INFO and QUERY_STRING.
		uriStr = params["SCRIPT_NAME"] + params["PATH_INFO"]
		s := params["QUERY_STRING"]
		if s != "" {
			uriStr += "?" + s
		}
	}

	// There's apparently a de-facto standard for this.
	// https://web.archive.org/web/20170105004655/http://docstore.mik.ua/orelly/linux/cgi/ch03_02.htm#ch03-35636
	if s := params["HTTPS"]; s == "on" || s == "ON" || s == "1" {
		r.TLS = &tls.ConnectionState{HandshakeComplete: true}
	}

	if r.Host != "" {
		// Hostname is provided, so we can reasonably construct a URL.
		rawurl := r.Host + uriStr
		if r.TLS == nil {
			rawurl = "http://" + rawurl
		} else {
			rawurl = "https://" + rawurl
		}
		url, err := url.Parse(rawurl)
		if err != nil {
			return nil, errors.New("cgi: failed to parse host and REQUEST_URI into a URL: " + rawurl)
		}
		r.URL = url
	}
	// Fallback logic if we don't have a Host header or the URL
	// failed to parse
	if r.URL == nil {
		url, err := url.Parse(uriStr)
		if err != nil {
			return nil, errors.New("cgi: failed to parse REQUEST_URI into a URL: " + uriStr)
		}
		r.URL = url
	}

	// Request.RemoteAddr has its port set by Go's standard http
	// server, so we do here too.
	remotePort, _ := strconv.Atoi(params["REMOTE_PORT"]) // zero if unset or invalid
	r.RemoteAddr = net.JoinHostPort(params["REMOTE_ADDR"], strconv.Itoa(remotePort))

	// 取出访问的文件
	scriptFileName := params["SCRIPT_FILENAME"]
	if scriptFileName != "" {
		r.Header.Add("FilePath", scriptFileName)
	}

	return r, nil
}
