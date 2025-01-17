package fastcgi

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
)

type rctype uint8

/*
	定义包的大小常量

contentlength用2个字节表示则最大为65535
paddinglenth 用1个字节表示则最大为255
*/
const (
	MAXCONTENT = 65535
	MAXPAD     = 255
)

// 消息头类型定义
const (
	FCGI_BEGIN_REQUEST     rctype = 1
	FCGI_ABORT_REQUEST     rctype = 2
	FCGI_END_REQUEST       rctype = 3
	FCGI_PARAMS            rctype = 4
	FCGI_STDIN             rctype = 5
	FCGI_STDOUT            rctype = 6
	FCGI_STDERR            rctype = 7
	FCGI_DATA              rctype = 8
	FCGI_GET_VALUES        rctype = 9
	FCGI_GET_VALUES_RESULT rctype = 10
	FCGI_UNKNOWN_TYPE      rctype = 11
)

// 是否保持连接
const flagKeepAlive = 1

// 角色
const (
	ROLE_FCGI_RESPONSE   = iota + 1 // 响应器
	ROLE_FCGI_AUTHORIZER            // 授权器
	ROLE_FCGI_FILTER                // 过滤器
)

// 请求完成描述
const (
	FCGI_REQUEST_COMPLETE = iota
	FCGI_CANT_MPX_CONN
	FCGI_OVERLOADED
	FCGI_UNKNOW_ROLE
)

type bufWriter struct {
	closer io.Closer
	*bufio.Writer
}

func (w *bufWriter) Close() error {
	if err := w.Writer.Flush(); err != nil {
		w.closer.Close()
		return err
	}
	return w.closer.Close()
}

func newWriter(c *Conn, recType rctype, reqId uint16) *bufWriter {
	s := &streamWriter{c: c, recType: recType, reqId: reqId}
	w := bufio.NewWriterSize(s, MAXCONTENT)
	return &bufWriter{s, w}
}

type streamWriter struct {
	c       *Conn
	recType rctype
	reqId   uint16
}

func (s *streamWriter) Write(b []byte) (int, error) {
	// 消息头 + 消息体
	nn := 0
	for len(b) > 0 {
		n := len(b)
		if n > MAXCONTENT {
			n = MAXCONTENT
		}
		if err := s.c.writeRecord(s.recType, s.reqId, b[:n]); err != nil {
			return nn, err
		}
		nn += n
		b = b[n:]
	}
	return nn, nil
}

func (s *streamWriter) Close() error {
	return s.c.writeRecord(s.recType, s.reqId, nil)
}

// 消息头结构体，8字节
type FCGI_Header struct {
	Version       uint8  // 版本
	Ftype         rctype // 类型
	RequestId     uint16 // 请求ID， 两个字节 对应 RequestId1 和 RequestId0
	ContentLength uint16 // 内容长度，两个字节 对应 ContentLengthB1 和 ContentLengthB0
	PaddingLength uint8  // 填充字节的长度
	Reserved      uint8  // 保留字节
}

func (h *FCGI_Header) init(ftype rctype, reqId uint16, contentLen uint16) {
	h.Version = 1
	h.Ftype = ftype
	h.RequestId = reqId
	h.ContentLength = contentLen
	h.PaddingLength = uint8(-contentLen & 7)
}

// ftype=1 消息体
type FCGI_BeginRequestBody struct {
	Role     uint16   // 角色，2字节 对应 RoleB1 RoleB0
	Flags    uint8    // 是否保持连接标记
	Reserved [5]uint8 // 保留字段
}

func (rb *FCGI_BeginRequestBody) read(content []byte) error {
	if len(content) != 8 {
		return errors.New("fcgi: invalid begin request record")
	}
	rb.Role = binary.BigEndian.Uint16(content)
	rb.Flags = content[2]
	return nil
}

// record
type Record struct {
	h   FCGI_Header
	buf [MAXCONTENT + MAXPAD]byte
}

// 读取一条record
func (r *Record) read(reader io.Reader) (err error) {
	// 通过大端法读取数据到header中
	if err = binary.Read(reader, binary.BigEndian, &r.h); err != nil {
		return err
	}
	// 判断版本
	if r.h.Version != 1 {
		return errors.New("invalid header version")
	}
	// 计算消息内容长度
	n := int(r.h.ContentLength) + int(r.h.PaddingLength)
	// 读取内容到buf中
	if _, err := io.ReadFull(reader, r.buf[:n]); err != nil {
		return err
	}
	return nil
}

// 返回内容
func (r *Record) content() []byte {
	return r.buf[:r.h.ContentLength]
}

var pad [MAXPAD]byte

type Conn struct {
	mutex sync.Mutex
	rwc   io.ReadWriteCloser

	buf bytes.Buffer
	h   FCGI_Header
}

func newConn(rwc io.ReadWriteCloser) *Conn {
	return &Conn{rwc: rwc}
}

func (c *Conn) Close() error {
	return c.rwc.Close()
}

// 写record
func (c *Conn) writeRecord(rtype rctype, reqId uint16, bytes []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.buf.Reset()
	c.h.init(rtype, reqId, uint16(len(bytes)))
	if err := binary.Write(&c.buf, binary.BigEndian, c.h); err != nil {
		return err
	}
	if _, err := c.buf.Write(bytes); err != nil {
		return err
	}
	if _, err := c.buf.Write(pad[:c.h.PaddingLength]); err != nil {
		return err
	}
	// 发送消息体
	_, err := c.rwc.Write(c.buf.Bytes())
	return err
}

// fcgi_end_request
func (c *Conn) withEndRequest(reqId uint16, appStatus uint32, protocolStatus uint8) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b, appStatus)
	b[4] = protocolStatus
	return c.writeRecord(FCGI_END_REQUEST, reqId, b)
}

// FcgiServer
type FcgiServer struct {
	addr     string
	listener net.Listener
	handler  http.Handler
}

func NewFcgiServer(addr string, handler http.Handler) *FcgiServer {
	srv := &FcgiServer{
		addr:     addr,
		listener: nil,
		handler:  handler,
	}
	return srv
}

func readSize(s []byte) (uint32, int) {
	if len(s) == 0 {
		return 0, 0
	}
	size, n := uint32(s[0]), 1
	// 如果第一个字节的最高位为1 则将用四个字节表示
	if size&(1<<7) != 0 {
		if len(s) < 4 {
			return 0, 0
		}
		n = 4
		size = binary.BigEndian.Uint32(s)
	}
	return size, n
}
