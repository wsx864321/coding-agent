package jsonrpc

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Transport 抽象 JSON-RPC 消息的帧协议
type Transport interface {
	// WriteFrame 将序列化后的 JSON body 按传输格式写入 writer
	WriteFrame(w io.Writer, body []byte) error
	// ReadLoop 从 reader 持续读取帧，每读到完整一帧就调用 handler
	// handler 返回 error 时停止读取
	ReadLoop(r io.Reader, handler func(body []byte) error) error
}

// ---------- Line-delimited (MCP) ----------

// LineTransport 是换行分隔的帧协议：每条 JSON 消息占一行，以 \n 结尾
type LineTransport struct{}

func (LineTransport) WriteFrame(w io.Writer, body []byte) error {
	data := make([]byte, len(body)+1)
	copy(data, body)
	data[len(body)] = '\n'
	_, err := w.Write(data)
	return err
}

func (LineTransport) ReadLoop(r io.Reader, handler func(body []byte) error) error {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read line: %w", err)
		}
		// 去掉尾部空白
		line = trimRight(line)
		if len(line) == 0 {
			continue
		}
		if err := handler(line); err != nil {
			return err
		}
	}
}

// ---------- Content-Length (LSP) ----------

// ContentLengthTransport 是 LSP 使用的 Content-Length 帧协议
type ContentLengthTransport struct{}

func (ContentLengthTransport) WriteFrame(w io.Writer, body []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := w.Write([]byte(header)); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

func (ContentLengthTransport) ReadLoop(r io.Reader, handler func(body []byte) error) error {
	reader := bufio.NewReader(r)
	for {
		header, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read header: %w", err)
		}
		header = strings.TrimSpace(header)
		if !strings.HasPrefix(header, "Content-Length: ") {
			continue
		}

		length, err := strconv.Atoi(strings.TrimPrefix(header, "Content-Length: "))
		if err != nil {
			continue
		}

		// 跳过 \r\n
		reader.ReadString('\n')

		body := make([]byte, length)
		if _, err := io.ReadFull(reader, body); err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		if err := handler(body); err != nil {
			return err
		}
	}
}

func trimRight(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}
