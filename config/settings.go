package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

const (
	// CacheSize default cache size 100M
	CacheSize = 100
	// RecordLimit output logreplay default limit
	RecordLimit = 10000
	// QPSLimit output logreplay qps limit
	QPSLimit = 10
	// EnvTest gateway test env
	EnvTest = "test"
	// EnvFormal gateway formal env
	EnvFormal = "formal"
	// FluxSwitchDefault flux switch default is close
	FluxSwitchDefault = "0"

	sizeLimit      = 33554432
	fileMaxSize    = 1099511627776
	copyBufferSize = 5242880
)

// MultiOption allows to specify multiple flags with same name and collects all values into array
type MultiOption []string

// String MultiOption to string method
func (h *MultiOption) String() string {
	return fmt.Sprint(*h)
}

// Set gets called multiple times for each flag with same name
func (h *MultiOption) Set(value string) error {
	*h = append(*h, value)
	return nil
}

// AppSettings is the struct of main configuration
type AppSettings struct {
	Verbose        int           `json:"verbose"`
	LogPath        string        `json:"log-path"`
	Stats          bool          `json:"stats"`
	OnlyOneProcess bool          `json:"only-one-process"`
	ExitAfter      time.Duration `json:"exit-after"`

	SplitOutput bool `json:"split-output"`

	InputDummy   MultiOption `json:"input-dummy"`
	OutputDummy  MultiOption
	OutputStdout bool `json:"output-stdout"`
	OutputNull   bool `json:"output-null"`

	InputTCP        MultiOption `json:"input-tcp"`
	InputTCPConfig  TCPInputConfig
	OutputTCP       MultiOption `json:"output-tcp"`
	OutputTCPConfig TCPOutputConfig
	OutputTCPStats  bool `json:"output-tcp-stats"`

	InputFile        MultiOption `json:"input-file"`
	InputFileLoop    bool        `json:"input-file-loop"`
	OutputFile       MultiOption `json:"output-file"`
	OutputFileConfig FileOutputConfig

	InputRAW MultiOption `json:"input_raw"`
	RAWInputConfig

	Middleware string `json:"middleware"`

	InputHTTP       MultiOption
	OutputHTTP      MultiOption `json:"output-http"`
	OutputLogReplay bool        `json:"output-logreplay"`
	PrettifyHTTP    bool        `json:"prettify-http"`

	OutputHTTPConfig      HTTPOutputConfig
	OutputLogReplayConfig LogReplayOutputConfig

	OutputBinary       MultiOption `json:"output-binary"`
	OutputBinaryConfig BinaryOutputConfig

	ModifierConfig HTTPModifierConfig

	InputUDP         MultiOption `json:"input-udp"`
	InputUDPConfig   UDPInputConfig
	InputKafkaConfig InputKafkaConfig
	KafkaTLSConfig   KafkaTLSConfig
}

// Settings holds Gor configuration
var Settings AppSettings

func usage() {
	fmt.Printf(`Gor is a simple http traffic replication tool written in Go.
Its main goal is to replay traffic from production servers to staging and dev environments.
Project page: https://github.com/buger/gor
Author: <Leonid Bugaev> leonsbox@gmail.com
Current Version: v%s\n`, VERSION)

	flag.PrintDefaults()
	os.Exit(2)
}

func init() {
	flag.Usage = usage
	flag.IntVar(&Settings.Verbose, "verbose", 0,
		"set the level of verbosity, if greater than zero then it will turn on debug output")
	flag.StringVar(&Settings.LogPath, "log-path", "", "path of log")
	flag.BoolVar(&Settings.Stats, "stats", false, "Turn on queue stats output")
	flag.BoolVar(&Settings.OnlyOneProcess, "only-one-process", false,
		"only one goreplay process can run in thin machine")

	// ExitAfter 设定退出的时间。
	setExitAfter()
	// input raw flags
	setRawInputFlags()
	// outputHTTPConfig
	setOutputHTTPConfig()
	// setOutputLogReplayConfig
	setOutputLogReplayConfig()
	// setOutputBinaryConfig
	setOutputBinaryConfig()
	// setModifierConfig
	setModifierConfig()
	// default values, using for tests
	Settings.OutputFileConfig.SizeLimit = sizeLimit
	Settings.OutputFileConfig.OutputFileMaxSize = fileMaxSize
	Settings.CopyBufferSize = copyBufferSize
	// set default config
	setDefault()
	// setInputUDPConfig
	setInputUDPConfig()

	fmt.Println("setting is init. verbose:", Settings.Verbose)
}

func setDefault() {
	if Settings.OutputFileConfig.SizeLimit < 1 {
		if err := Settings.OutputFileConfig.SizeLimit.Set("32mb"); err != nil {
			log.Printf("err: %v", err)
		}
	}

	if Settings.OutputFileConfig.OutputFileMaxSize < 1 {
		if err := Settings.OutputFileConfig.OutputFileMaxSize.Set("1tb"); err != nil {
			log.Printf("err: %v", err)
		}
	}

	if Settings.CopyBufferSize < 1 {
		if err := Settings.CopyBufferSize.Set("5mb"); err != nil {
			log.Printf("err: %v", err)
		}
	}

	if Settings.Logreplay {
		if Settings.LogreplaySampleRate < 0 || Settings.LogreplaySampleRate > 16 {
			log.Printf("input-raw-logreplay-sample-rate 的取值范围是 [0, 16]")
		}
	}
}

func setExitAfter() {
	flag.DurationVar(&Settings.ExitAfter, "exit-after", -1, "exit after specified duration")

	flag.BoolVar(&Settings.SplitOutput, "split-output", false,
		"By default each output gets same traffic. "+
			"If set to `true` it splits traffic equally among all outputs.")

	flag.Var(&Settings.InputDummy, "input-dummy", "Used for testing outputs. "+
		"Emits 'Get /' request every 1s")
	flag.BoolVar(&Settings.OutputStdout, "output-stdout", false,
		"Used for testing inputs. Just prints to console data coming from inputs.")
	flag.BoolVar(&Settings.OutputNull, "output-null", false,
		"Used for testing inputs. Drops all requests.")

	flag.Var(&Settings.InputTCP, "input-tcp",
		"Used for internal communication between Gor instances. Example: \n\t"+
			"# Receive requests from other Gor instances on 28020 port, and redirect output to staging\n\t"+
			"gor --input-tcp :28020 --output-http staging.com")
	flag.BoolVar(&Settings.InputTCPConfig.Secure, "input-tcp-secure", false,
		"Turn on TLS security. Do not forget to specify certificate and key files.")
	flag.StringVar(&Settings.InputTCPConfig.CertificatePath, "input-tcp-certificate", "",
		"Path to PEM encoded certificate file. Used when TLS turned on.")
	flag.StringVar(&Settings.InputTCPConfig.KeyPath, "input-tcp-certificate-key", "",

		"Path to PEM encoded certificate key file. Used when TLS turned on.")
	flag.Var(&Settings.OutputTCP, "output-tcp",
		"Used for internal communication between Gor instances. Example: \n\t"+
			"# Listen for requests on 80 port and "+
			"forward them to other Gor instance on 28020 port\n\t"+
			"gor --input-raw :80 --output-tcp replay.local:28020")
	flag.BoolVar(&Settings.OutputTCPConfig.Secure, "output-tcp-secure", false,
		"Use TLS secure connection. --input-file on another end should have TLS turned on as well.")
	flag.BoolVar(&Settings.OutputTCPConfig.SkipVerify, "output-tcp-skip-verify", false,
		"Don't verify hostname on TLS secure connection.")
	flag.BoolVar(&Settings.OutputTCPConfig.Sticky, "output-tcp-sticky", false,
		"Use Sticky connection. Request/Response with same ID will be sent to the same connection.")
	flag.IntVar(&Settings.OutputTCPConfig.Workers, "output-tcp-workers", 10,
		"Number of parallel tcp connections, default is 10")
	flag.BoolVar(&Settings.OutputTCPStats, "output-tcp-stats", false,
		"Report TCP output queue stats to console every 5 seconds.")

	flag.Var(&Settings.InputFile, "input-file",
		"Read requests from file: \n\tgor --input-file ./requests.gor --output-http staging.com")
	flag.BoolVar(&Settings.InputFileLoop, "input-file-loop", false,
		"Loop input files, useful for performance testing.")

	flag.Var(&Settings.OutputFile, "output-file",
		"Write incoming requests to file: \n\tgor --input-raw :80 --output-file ./requests.gor")
	flag.DurationVar(&Settings.OutputFileConfig.FlushInterval, "output-file-flush-interval", time.Second,
		"Interval for forcing buffer flush to the file, default: 1s.")
	flag.BoolVar(&Settings.OutputFileConfig.Append, "output-file-append", false,
		"The flushed chunk is appended to existence file or not. ")
	flag.Var(&Settings.OutputFileConfig.SizeLimit, "output-file-size-limit",
		"Size of each chunk. Default: 32mb")
	flag.IntVar(&Settings.OutputFileConfig.QueueLimit, "output-file-queue-limit", 256,
		"The length of the chunk queue. Default: 256")
	flag.Var(&Settings.OutputFileConfig.OutputFileMaxSize, "output-file-max-size-limit",
		"Max size of output file, Default: 1TB")

	flag.StringVar(&Settings.OutputFileConfig.BufferPath, "output-file-buffer", "/tmp",
		"The path for temporary storing current buffer: \n\t"+
			"gor --input-raw :80 "+
			"--output-file s3://mybucket/logs/%Y-%m-%d.gz --output-file-buffer /mnt/logs")

	flag.BoolVar(&Settings.PrettifyHTTP, "prettify-http", false,
		"If enabled, will automatically decode requests and responses with: "+
			"Content-Encoding: gzip and Transfer-Encoding: chunked. "+
			"Useful for debugging, in conjuction with --output-stdout")
}

func setRawInputFlags() {
	flag.Var(&Settings.InputRAW, "input-raw",
		"Capture traffic from given port (use RAW sockets and require *sudo* access):\n\t"+
			"# Capture traffic from 8080 port\n\tgor --input-raw :8080 --output-http staging.com")
	flag.BoolVar(&Settings.TrackResponse, "input-raw-track-response", false,
		"If turned on Gor will track responses in addition to requests, "+
			"and they will be available to middleware and file output.")
	flag.Var(&Settings.Engine, "input-raw-engine",
		"Intercept traffic using `libpcap` (default), `raw_socket` or `pcap_file`")
	flag.StringVar(&Settings.Protocol, "input-raw-protocol", "",
		"Specify application protocol of intercepted traffic. ")
	flag.StringVar(&Settings.RecordMsgType, "input-raw-record-msg-type", "",
		"Record message type: req or rsp")
	flag.StringVar(&Settings.RecordMsgCmd, "input-raw-record-msg-cmd", "",
		"Record message cmd")
	flag.StringVar(&Settings.RealIPHeader, "input-raw-realip-header", "",
		"If not blank, injects header with given name and real IP value to the request payload. "+
			"Usually this header should be named: X-Real-IP")
	flag.DurationVar(&Settings.Expire, "input-raw-expire", time.Second*2,
		"How much it should wait for the last TCP packet, till consider that TCP message complete.")
	flag.StringVar(&Settings.BPFFilter, "input-raw-bpf-filter", "",
		"BPF filter to write custom expressions. "+
			"Can be useful in case of non standard network interfaces like tunneling or SPAN port. "+
			"Example: --input-raw-bpf-filter 'dst port 80'")
	flag.StringVar(&Settings.TimestampType, "input-raw-timestamp-type", "",
		"Possible values: PCAP_TSTAMP_HOST, PCAP_TSTAMP_HOST_LOWPREC, PCAP_TSTAMP_HOST_HIPREC, "+
			"PCAP_TSTAMP_ADAPTER, PCAP_TSTAMP_ADAPTER_UNSYNCED."+
			" This values not supported on all systems, "+
			"GoReplay will tell you available values of you put wrong one.")
	flag.Var(&Settings.CopyBufferSize, "copy-buffer-size",
		"Set the buffer size for an individual request (default 5MB)")
	flag.BoolVar(&Settings.Snaplen, "input-raw-override-snaplen", false,
		"Override the capture snaplen to be 64k. Required for some Virtualized environments")
	flag.DurationVar(&Settings.BufferTimeout, "input-raw-buffer-timeout", 0,
		"set the pcap timeout. for immediate mode don't set this flag")
	flag.Var(&Settings.BufferSize, "input-raw-buffer-size",
		"Controls size of the OS buffer which holds packets until they dispatched. "+
			"Default value depends by system: in Linux around 2MB. "+
			"If you see big package drop, increase this value.")
	flag.BoolVar(&Settings.Promiscuous, "input-raw-promisc", false, "enable promiscuous mode")
	flag.BoolVar(&Settings.Monitor, "input-raw-monitor", false, "enable RF monitor mode")
	flag.BoolVar(&Settings.Stats, "input-raw-stats", false,
		"enable stats generator on raw TCP messages")
	flag.BoolVar(&Settings.Logreplay, "input-raw-logreplay", false, "enable logreplay")
	flag.IntVar(&Settings.LogreplaySampleRate, "input-raw-logreplay-sample-rate", 16,
		"采样率，取值从 0 到 16，分别代表 1/16 到 1 的采样率")
	flag.BoolVar(&Settings.AutoSelectIP, "input-raw-auto-select-ip", false,
		"auto select first ip if multiple exists")
	flag.StringVar(&Settings.Middleware, "middleware", "",
		"Used for modifying traffic using external command")

	flag.Var(&Settings.OutputHTTP, "output-http", "Forwards incoming requests to given http address.\n\t"+
		"# Redirect all incoming requests to staging.com address \n\t"+
		"gor --input-raw :80 --output-http http://staging.com")
	flag.StringVar(&Settings.SelectHost, "input-raw-select-host", "", "select the traffic of the specified host.\n\t")
	flag.StringVar(&Settings.AspectInfo, "input-raw-aspect-info", "", "the traffic of aspect\n\t")
	flag.StringVar(&Settings.InputKafkaConfig.Host, "input-kafka-host", "",
		"Send request and response stats to Kafka:\n\t"+
			"gor --output-stdout --input-kafka-host '192.168.0.1:9092,192.168.0.2:9092'")
	flag.StringVar(&Settings.InputKafkaConfig.Topic, "input-kafka-topic", "",
		"Send request and response stats to Kafka:"+
			"\n\tgor --output-stdout --input-kafka-topic 'kafka-log'")
	flag.BoolVar(&Settings.InputKafkaConfig.UseJSON, "input-kafka-json-format",
		false, "If turned on, it will assume that messages "+
			"coming in JSON format rather than  GoReplay text format.")

}

func setOutputHTTPConfig() {
	flag.Var(&Settings.OutputHTTPConfig.BufferSize, "output-http-response-buffer",
		"HTTP response buffer size, all data after this size will be discarded.")
	flag.IntVar(&Settings.OutputHTTPConfig.WorkersMin, "output-http-workers-min", 0,
		"Gor uses dynamic worker scaling. Enter a number to set a minimum number of workers. default = 1.")
	flag.IntVar(&Settings.OutputHTTPConfig.WorkersMax, "output-http-workers", 0,
		"Gor uses dynamic worker scaling. "+
			"Enter a number to set a maximum number of workers. default = 0 = unlimited.")
	flag.IntVar(&Settings.OutputHTTPConfig.QueueLen, "output-http-queue-len", 1000,
		"Number of requests that can be queued for output, if all workers are busy. default = 1000")
	flag.BoolVar(&Settings.OutputHTTPConfig.SkipVerify, "output-http-skip-verify", false,
		"Don't verify hostname on TLS secure connection.")
	flag.DurationVar(&Settings.OutputHTTPConfig.WorkerTimeout, "output-http-worker-timeout", 2*time.Second,
		"Duration to rollback idle workers.")

	flag.IntVar(&Settings.OutputHTTPConfig.RedirectLimit, "output-http-redirects", 0,
		"Enable how often redirects should be followed.")
	flag.DurationVar(&Settings.OutputHTTPConfig.Timeout, "output-http-timeout", 5*time.Second,
		"Specify HTTP request/response timeout. By default 5s. Example: --output-http-timeout 30s")
	flag.BoolVar(&Settings.OutputHTTPConfig.TrackResponses, "output-http-track-response", false,
		"If turned on, HTTP output responses will be set to all outputs like stdout, file and etc.")

	flag.BoolVar(&Settings.OutputHTTPConfig.Stats, "output-http-stats", false,
		"Report http output queue stats to console every N milliseconds. See output-http-stats-ms")
	flag.IntVar(&Settings.OutputHTTPConfig.StatsMs, "output-http-stats-ms", 5000,
		"Report http output queue stats to console every N milliseconds. default: 5000")
	flag.BoolVar(&Settings.OutputHTTPConfig.OriginalHost, "http-original-host", false,
		"Normally gor replaces the Host http header with the host supplied with --output-http.  "+
			"This option disables that behavior, preserving the original Host header.")
	flag.StringVar(&Settings.OutputHTTPConfig.ElasticSearch, "output-http-elasticsearch", "",
		"Send request and response stats to ElasticSearch:\n\t"+
			"gor --input-raw :8080 --output-http staging.com "+
			"--output-http-elasticsearch 'es_host:api_port/index_name'")
}

func setOutputLogReplayConfig() {
	flag.BoolVar(&Settings.OutputLogReplay, "output-logreplay", false,
		"Forwards incoming requests to logreplay.\n\t"+
			"# Redirect all incoming requests to logreplay \n\tgor --input-raw :80 --output-logreplay")
	flag.StringVar(&Settings.OutputLogReplayConfig.ModuleID, "output-logreplay-moduleid", "",
		"LogReplay moduleid")
	flag.StringVar(&Settings.OutputLogReplayConfig.CommitID, "output-logreplay-commitid", "",
		"LogReplay commitid")
	flag.StringVar(&Settings.OutputLogReplayConfig.APPID, "output-logreplay-appid", "",
		"LogReplay API auth appid")
	flag.StringVar(&Settings.OutputLogReplayConfig.APPKey, "output-logreplay-appkey", "",
		"LogReplay API auth appkey")
	flag.IntVar(&Settings.OutputLogReplayConfig.CacheSize, "output-logreplay-cache-size", CacheSize,
		"freecache to cache request. By default 100M. Example: --output-logreplay-cache-size 50")
	flag.DurationVar(&Settings.OutputLogReplayConfig.Timeout, "output-logreplay-timeout", 0,
		"Specify LogReplay API HTTP request/response timeout. "+
			"By default 1s. Example: --output-logreplay-timeout 30s")
	flag.BoolVar(&Settings.OutputLogReplayConfig.TrackResponses, "output-logreplay-track-response", false,
		"If turned on, LogReplay output responses will be set to all outputs like stdout, file and etc.")
	flag.IntVar(&Settings.OutputLogReplayConfig.Workers, "output-logreplay-workers", 0,
		"Gor uses dynamic worker scaling by default.  Enter a number to run a set number of workers.")
	flag.StringVar(&Settings.OutputLogReplayConfig.Target, "output-logreplay-target", "",
		"LogReplay target replay address")
	flag.DurationVar(&Settings.OutputLogReplayConfig.TargetTimeout, "output-logreplay-target-timeout", 0,
		"LogReplay target replay timeout")
	flag.IntVar(&Settings.OutputLogReplayConfig.RecordLimit, "output-logreplay-record-limit", RecordLimit,
		"LogReplay record limit")
	flag.StringVar(&Settings.OutputLogReplayConfig.ProtocolServiceName,
		"output-logreplay-protocol-service-name", "",
		"协议绑定的 serviceName，如果不配置，"+
			"会默认取 logreplay 上注册的 appName.ModuleName")
	flag.StringVar(&Settings.OutputLogReplayConfig.RealServerName,
		"output-logreplay-real-service-name", "",
		"对应服务地址 为空亦可")
	flag.StringVar(&Settings.OutputLogReplayConfig.RecordTagInfo,
		"output-logreplay-record-tags", "",
		"录制流量tag信息，标记流量")
	flag.StringVar(&Settings.OutputLogReplayConfig.LogreplayTaskID,
		"output-logreplay-task-id", "",
		"录制任务ID")
	flag.IntVar(&Settings.OutputLogReplayConfig.QPSLimit, "output-logreplay-qps-limit", QPSLimit,
		"LogReplay qps limit, default value is 10, max value is 100")
	flag.StringVar(&Settings.OutputLogReplayConfig.Env, "output-logreplay-env", EnvFormal,
		"LogReplay gateway env")
	flag.StringVar(&Settings.OutputLogReplayConfig.FluxSwitch, "output-flux-switch", FluxSwitchDefault,
		"flux switch")
	flag.StringVar(&Settings.OutputLogReplayConfig.GrpcReplayMethodName,
		"output-logreplay-grpc-method-name", "", "grpc 边录制边回放指定的回放的方法名称，为空则不做拦截")
	flag.StringVar(&Settings.OutputLogReplayConfig.Business,
		"output-logreplay-grpc-business-name", "", "使用goreplay业务名称，属于业务定制逻辑，不清楚的不需要填写")
	flag.Var(&Settings.OutputLogReplayConfig.RequestReWrite, "output-logreplay-request-rewrite",
		"Rewrite the request based on a source:target mapping. For example: /v1/user/([^\\/]+)/ping:/v2/user/$1/ping")
	flag.Var(&Settings.OutputLogReplayConfig.AllowRequest, "output-logreplay-allow-request",
		"Filter matched request with the regexp. Anything else will be dropped")
}

func setOutputBinaryConfig() {
	flag.Var(&Settings.OutputBinary, "output-binary",
		"Forwards incoming binary payloads to given address.\n\t"+
			"# Redirect all incoming requests to staging.com address \n\t"+
			"gor --input-raw :80 --input-raw-protocol binary --output-binary staging.com:80")
	flag.Var(&Settings.OutputBinaryConfig.BufferSize, "output-tcp-response-buffer",
		"TCP response buffer size, all data after this size will be discarded.")
	flag.IntVar(&Settings.OutputBinaryConfig.Workers, "output-binary-workers", 0,
		"Gor uses dynamic worker scaling by default.  Enter a number to run a set number of workers.")
	flag.DurationVar(&Settings.OutputBinaryConfig.Timeout, "output-binary-timeout", 0,
		"Specify HTTP request/response timeout. By default 5s. Example: --output-binary-timeout 30s")
	flag.BoolVar(&Settings.OutputBinaryConfig.TrackResponses, "output-binary-track-response", false,
		"If turned on, Binary output responses will be set to all outputs like stdout, file and etc.")

	flag.BoolVar(&Settings.OutputBinaryConfig.Debug, "output-binary-debug", false,
		"Enables binary debug output.")
}

func setModifierConfig() {
	flag.Var(&Settings.ModifierConfig.Headers, "http-set-header",
		"Inject additional headers to http reqest:\n\t"+
			"gor --input-raw :8080 --output-http staging.com --http-set-header 'User-Agent: Gor'")
	flag.Var(&Settings.ModifierConfig.HeaderRewrite, "http-rewrite-header",
		"Rewrite the request header based on a mapping:\n\t"+
			"gor --input-raw :8080 --output-http staging.com "+
			"--http-rewrite-header Host: (.*).example.com,$1.beta.example.com")
	flag.Var(&Settings.ModifierConfig.Params, "http-set-param",
		"Set request url param, if param already exists it will be overwritten:\n\t"+
			"gor --input-raw :8080 --output-http staging.com --http-set-param api_key=1")
	flag.Var(&Settings.ModifierConfig.Methods, "http-allow-method",
		"Whitelist of HTTP methods to replay. Anything else will be dropped:\n\t"+
			"gor --input-raw :8080 --output-http staging.com"+
			" --http-allow-method GET --http-allow-method OPTIONS")
	flag.Var(&Settings.ModifierConfig.URLRegexp, "http-allow-url",
		"A regexp to match requests against. "+
			"Filter get matched against full url with domain. Anything else will be dropped:\n\t "+
			"gor --input-raw :8080 --output-http staging.com --http-allow-url ^www.")
	flag.Var(&Settings.ModifierConfig.URLNegativeRegexp, "http-disallow-url",
		"A regexp to match requests against. Filter get matched against full url with domain. "+
			"Anything else will be forwarded:\n\t "+
			"gor --input-raw :8080 --output-http staging.com --http-disallow-url ^www.")
	flag.Var(&Settings.ModifierConfig.URLRewrite, "http-rewrite-url",
		"Rewrite the request url based on a mapping:\n\t"+
			"gor --input-raw :8080 --output-http staging.com "+
			"--http-rewrite-url /v1/user/([^\\/]+)/ping:/v2/user/$1/ping")
	flag.Var(&Settings.ModifierConfig.HeaderFilters, "http-allow-header",
		"A regexp to match a specific header against."+
			" Requests with non-matching headers will be dropped:\n\t "+
			"gor --input-raw :8080 --output-http staging.com --http-allow-header api-version:^v1")
	flag.Var(&Settings.ModifierConfig.HeaderNegativeFilters, "http-disallow-header",
		"A regexp to match a specific header against. "+
			"Requests with matching headers will be dropped:\n\t "+
			"gor --input-raw :8080 --output-http staging.com --http-disallow-header"+
			" \"User-Agent: Replayed by Gor\"")
	flag.Var(&Settings.ModifierConfig.HeaderBasicAuthFilters, "http-basic-auth-filter",
		"A regexp to match the decoded basic auth string against. "+
			"Requests with non-matching headers will be dropped:\n\t "+
			"gor --input-raw :8080 --output-http staging.com"+
			" --http-basic-auth-filter \"^customer[0-9].*\"")
	flag.Var(&Settings.ModifierConfig.HeaderHashFilters, "http-header-limiter",
		"Takes a fraction of requests, consistently taking or rejecting a request "+
			"based on the FNV32-1A hash of a specific header:\n\t "+
			"gor --input-raw :8080 --output-http staging.com --http-header-limiter user-id:25%")
	flag.Var(&Settings.ModifierConfig.ParamHashFilters, "http-param-limiter",
		"Takes a fraction of requests, consistently taking or rejecting a request "+
			"based on the FNV32-1A hash of a specific GET param:\n\t "+
			"gor --input-raw :8080 --output-http staging.com --http-param-limiter user_id:25%")
}

func setInputUDPConfig() {
	flag.Var(&Settings.InputUDP, "input-udp",
		"Capture traffic from given port (use RAW sockets and require *sudo* access):\n\t"+
			"# Capture traffic from 8080 port\n\tgor --input-udp :8080 --output-stdout")
	flag.BoolVar(&Settings.InputUDPConfig.TrackResponse, "input-udp-track-response", true,
		"If turned on gorepaly-udp will track responses in addition to requests")
	flag.StringVar(&Settings.InputUDPConfig.Protocol, "input-udp-protocol", "",
		"Specify application protocol of intercepted traffic.")

}
