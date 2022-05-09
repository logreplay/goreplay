package analyze

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
)

// HTTP http协议解析器
type HTTP struct {
}

// GetServiceCmd 获取命令字
func (h *HTTP) GetServiceCmd(data []byte) (string, error) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		return "", fmt.Errorf("decodeHTTPRequest error: %v", err)
	}
	return req.RequestURI, nil
}
