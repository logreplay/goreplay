package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"goreplay/size"
)

// FileOutputConfig file output configuration
type FileOutputConfig struct {
	FlushInterval     time.Duration `json:"output-file-flush-interval"` // FlushInterval write file flush duration
	SizeLimit         size.Size     `json:"output-file-size-limit"`     // SizeLimit output file chunk size limit
	OutputFileMaxSize size.Size     `json:"output-file-max-size-limit"` // OutputFileMaxSize max size limit
	QueueLimit        int           `json:"output-file-queue-limit"`    // QueueLimit file write queue len limit
	Append            bool          `json:"output-file-append"`         //
	BufferPath        string        `json:"output-file-buffer"`
	OnClose           func(string)
}

// TCPOutputConfig tcp output configuration
type TCPOutputConfig struct {
	Secure     bool `json:"output-tcp-secure"`
	Sticky     bool `json:"output-tcp-sticky"`
	SkipVerify bool `json:"output-tcp-skip-verify"`
	Workers    int  `json:"output-tcp-workers"`
}

// HTTPOutputConfig struct for holding http output configuration
type HTTPOutputConfig struct {
	TrackResponses bool          `json:"output-http-track-response"`
	Stats          bool          `json:"output-http-stats"`
	OriginalHost   bool          `json:"output-http-original-host"`
	RedirectLimit  int           `json:"output-http-redirect-limit"`
	WorkersMin     int           `json:"output-http-workers-min"`
	WorkersMax     int           `json:"output-http-workers"`
	StatsMs        int           `json:"output-http-stats-ms"`
	QueueLen       int           `json:"output-http-queue-len"`
	ElasticSearch  string        `json:"output-http-elasticsearch"`
	Timeout        time.Duration `json:"output-http-timeout"`
	WorkerTimeout  time.Duration `json:"output-http-worker-timeout"`
	BufferSize     size.Size     `json:"output-http-response-buffer"`
	SkipVerify     bool          `json:"output-http-skip-verify"`
	RawURL         string
	URL            *url.URL
}

// LogReplayOutputConfig struct for logreplay output configuration
type LogReplayOutputConfig struct {
	Protocol       string        // 录制的协议，grpc, http 等，必填
	SerializeType  string        `json:"output-logreplay-serialize-type"` // SerializeType 数据包业务数据序列化类型：pb
	ModuleID       string        `json:"output-logreplay-moduleid"`       // ModuleID，必填
	CommitID       string        `json:"output-logreplay-commitid"`       // CommitID，必填
	APPID          string        `json:"output-logreplay-appid"`          // APPID，网关的 appID，必填
	APPKey         string        `json:"output-logreplay-appkey"`         // APPKey 网关的 appKey，必填
	Env            string        `json:"output-logreplay-env"`            // Env logreplay 的网关 env，默认是 formal
	CacheSize      int           `json:"output-logreplay-cache-size"`     // CacheSize 占用的缓存大小，默认 100M，最大 200M
	Timeout        time.Duration `json:"output-logreplay-timeout"`        // Timeout 请求 logreplay 后台的超时时间，默认 1s
	TrackResponses bool          `json:"output-logreplay-track-response"` // TrackResponses 参考 input-raw-track-response
	Workers        int           `json:"output-logreplay-workers"`        // Workers logreplay 处理数据的协程数，默认是 1
	Target         string        `json:"output-logreplay-target"`         // Target 边录制边回放时的 回放目标地址
	TargetTimeout  time.Duration `json:"output-logreplay-target-timeout"` // TargetTimeout 边录制边回放时的 回放超时时间
	RecordLimit    int           `json:"output-logreplay-record-limit"`   // RecordLimit 达到录制的上限退出，默认 10000
	QPSLimit       int           `json:"output-logreplay-qps-limit"`      // QPSLimit 录制 qps 限制，默认 qps 为 10
	// ProtocolServiceName 如果不配置，会默认取 logreplay 上注册的 appName.ModuleName
	ProtocolServiceName string `json:"output-logreplay-protocol-service-name"`
	// RealServerName 对应服务地址
	RealServerName string `json:"output-logreplay-real-service-name"`
	// FluxSwitch 流量生成用例开关
	FluxSwitch string `json:"output-flux-switch"`
	// GrpcReplayMethodName Grpc 协议边录制边回放指定的方法名称，开启边录制边回放（指定 target，并且是 grpc 协议）才会生效，（可多个值输入，英文逗号分割）
	GrpcReplayMethodName string `json:"output-logreplay-grpc-method-name"`
	// RecordTagInfo 录制 tag 信息
	RecordTagInfo string `json:"output-logreplay-record-tags"`
	// Business 业务线名称
	Business string `json:"output-logreplay-grpc-business-name"`
	// LogreplayTaskID goreplay进程的任务ID
	LogreplayTaskID string            `json:"output-logreplay-task-id"`
	RequestReWrite  RequestRewriteMap `json:"output-logreplay-request-rewrite"` // rewrite the request
	AllowRequest    RequestRegexp     `json:"output-logreplay-allow-request"`   // filter matched request
	GatewayAddr  string `json:"output-logreplay-gateway"` // report addr, e.g ip:port
	ReportURLPath string `json:"output-logreplay-report-url"` // report url path
	HeartbeatURLPath string `json:"output-logreplay-heartbeat-url"` // heartbeat url path
}

// BinaryOutputConfig struct for holding binary output configuration
type BinaryOutputConfig struct {
	Workers        int           `json:"output-binary-workers"` // Workers
	Timeout        time.Duration `json:"output-binary-timeout"`
	BufferSize     size.Size     `json:"output-tcp-response-buffer"`
	Debug          bool          `json:"output-binary-debug"`
	TrackResponses bool          `json:"output-binary-track-response"`
}

type requestRewrite struct {
	Regexp *regexp.Regexp
	Target []byte
}

// RequestRewriteMap holds regexp and data to modify request
type RequestRewriteMap []requestRewrite

// String RequestRewriteMap to string method
func (r *RequestRewriteMap) String() string {
	return fmt.Sprint(*r)
}

// Set method to implement flags.Value
func (r *RequestRewriteMap) Set(value string) error {
	pairs := strings.Split(value, ",") // multiple pairs in value are seperated by comma
	for _, pair := range pairs {
		pairArr := strings.SplitN(pair, ":", 2) // pair is in format source:target
		if len(pairArr) < 2 {
			return errors.New("need both src and target, colon-delimited (ex. /a:/b)")
		}
		regularExp, err := regexp.Compile(pairArr[0])
		if err != nil {
			return err
		}
		*r = append(*r, requestRewrite{Regexp: regularExp, Target: []byte(pairArr[1])})
	}
	return nil
}

type httpRegexp struct {
	Regexp *regexp.Regexp
}

// RequestRegexp a slice of regexp to match request
type RequestRegexp []httpRegexp

// String RequestRegexp to string method
func (r *RequestRegexp) String() string {
	return fmt.Sprint(*r)
}

// Set method to implement flags.Value
func (r *RequestRegexp) Set(value string) error {
	regularExp, err := regexp.Compile(value)
	if err != nil {
		return err
	}
	*r = append(*r, httpRegexp{Regexp: regularExp})
	return nil
}
