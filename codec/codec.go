// Package codec 解析请求头, 获取服务名, 接口名等信息
package codec

import "sync"

var (
	headCodecs = make(map[string]HeaderCodecBuilder)
	lock       sync.RWMutex
)

// HeaderCodec 协议头解析
//go:generate mockery --name HeaderCodec
type HeaderCodec interface {
	// Decode 获取到请求头结构体
	Decode(reqBuf []byte, connectionID string) (ProtocolHeader, error)
}

// ProtocolHeader 解析返回的头部信息
type ProtocolHeader struct {
	ServiceName   string            // 服务名
	APIName       string            // 获取接口名
	MethodName    string            // 方法名
	InterfaceName string            // 接口名
	CusTraceID    string            // 用户自定义traceID
	Properties    map[string]string // 业务自定义属性
}

// HeaderCodecBuilder HeaderCodec的建造者
type HeaderCodecBuilder interface {
	// New new一个HeaderCodec
	New() HeaderCodec
}

// RegisterHeaderCodec 注册请求头解析器
func RegisterHeaderCodec(proto string, headerCodecBuilder HeaderCodecBuilder) {
	lock.Lock()
	headCodecs[proto] = headerCodecBuilder
	lock.Unlock()
}

// GetHeaderCodec 获取请求头解析器
func GetHeaderCodec(proto string) HeaderCodec {
	lock.RLock()
	defer lock.RUnlock()

	if b, ok := headCodecs[proto]; ok {
		return b.New()
	}

	return &emptyHeaderCodec{}
}
