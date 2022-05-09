package plugins

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"goreplay/byteutils"
	"goreplay/client"
	"goreplay/codec"
	"goreplay/config"
	"goreplay/errors"
	"goreplay/logger"
	"goreplay/logreplay"
	"goreplay/protocol"
	"goreplay/utils/gateway"

	"github.com/coocood/freecache"
	jsoniter "github.com/json-iterator/go"
	"golang.org/x/net/http2"
)

const (
	freeCacheExpired = 60
	reportType       = "goReplay"
	replayType       = "replay"
	cacheSizeMin     = 100
	recordLimit      = 10000
	defaultQPSLimit  = 10
	trueStr          = "true"

	goreplay              = "goreplay"
	localhost             = "127.0.0.1"
	goReplayRequestKey    = "req_%s"
	goReplayReplayRespKey = "replay_rsp_%s"
	goReplayReqHeaderKey  = "req_header_%s"
)

// SendError 发送错误类型
type SendError int

const (
	emptySettingFrame = "000000040000000000"
)

// LogReplayOutput plugin manage pool of workers which send request to logreplay server
// By default workers pool is dynamic and starts with 1 worker or workerMin workers
// You can specify maximum number of workers using `--output-logreplay-workers`
type LogReplayOutput struct {
	address                                string
	conf                                   *config.LogReplayOutputConfig
	tcpClient                              *client.TCPClient
	cache                                  *freecache.Cache
	buf                                    []chan *Message
	responses                              chan *response
	stop                                   chan bool // Channel used only to indicate goroutine should shutdown
	target                                 string
	curQPS                                 uint32
	taskID                                 uint32
	success, dialFail, writeFail, readFail uint32
	reportBuf                              chan logreplay.ReportItem
	lastSampleTime                         int64
	json                                   jsoniter.API
	replayBuf                              chan replayParam
	targetList                             []string // 边录制边回放目标地址
	counter                                *logreplay.Counter
	RequestReWriteList                     []string
	AllowRequest                           string
}

// NewLogReplayOutput constructor for LogReplayOutput
// Initialize workers
func NewLogReplayOutput(address string, conf *config.LogReplayOutputConfig) PluginReadWriter {
	o := new(LogReplayOutput)
	o.json = jsoniter.ConfigCompatibleWithStandardLibrary
	o.address = localhost
	instanceName, err := getEnvInfo()
	if err == nil {
		o.address = instanceName
	}

	checkLogReplayConfig(conf)

	o.target = conf.Target
	o.counter = logreplay.NewCounter()
	o.conf = conf
	if o.conf.TrackResponses {
		o.responses = make(chan *response, o.conf.Workers)
	}

	if o.conf.TargetTimeout <= 0 {
		o.conf.TargetTimeout = time.Second
		logger.Info("[LOGREPLAY-OUTPUT] targetTimeout default value : ", o.conf.TargetTimeout)
	}

	if conf.Target != "" {
		o.targetList = strings.Split(o.conf.Target, ",")

		tcpClient := client.NewTCPClient(conf.Target, &client.TCPClientConfig{
			Debug:   true,
			Timeout: o.conf.TargetTimeout,
		})
		o.tcpClient = tcpClient
	}

	o.reportBuf = make(chan logreplay.ReportItem)
	o.replayBuf = make(chan replayParam)
	o.stop = make(chan bool)
	o.cache = freecache.NewCache(conf.CacheSize * 1024 * 1024)

	o.buf = make([]chan *Message, o.conf.Workers)
	for i := 0; i < o.conf.Workers; i++ {
		o.buf[i] = make(chan *Message, 100)
		go o.startWorker(i)
	}

	for i := 0; i < 5; i++ {
		go o.startReporter()
	}

	// 开启回放worker
	go o.startRelay()

	// 上报数据
	o.doReport()

	return o
}

func checkLogReplayConfig(conf *config.LogReplayOutputConfig) {
	if conf.ModuleID == "" {
		logger.Fatal("using logreplay require moduleid: --output-logreplay-moduleid")
		return
	}

	if conf.APPID == "" {
		logger.Fatal("using logreplay require appid: --output-logreplay-appid")
		return
	}

	if conf.APPKey == "" {
		logger.Fatal("using logreplay require appkey: --output-logreplay-appkey")
		return
	}

	if conf.CommitID == "" {
		logger.Fatal("using logreplay require commitID(or version of protocol): --output-logreplay-commitid")
		return
	}
	checkLogreplayOptionalConfig(conf)
}

func checkLogreplayOptionalConfig(conf *config.LogReplayOutputConfig) {
	if conf.Target != "" {
		for _, v := range strings.Split(conf.Target, ",") {
			checkAddress(v)
		}
	}

	if conf.Env == "" {
		conf.Env = config.EnvFormal
	}

	if conf.Env != config.EnvFormal {
		conf.Env = config.EnvTest
		logger.Info("[LOGREPLAY-OUTPUT] env default value : ", conf.Env)
	}

	if conf.Timeout < time.Millisecond*100 {
		conf.Timeout = time.Second
		logger.Info("[LOGREPLAY-OUTPUT] timeout default value : ", conf.Timeout)
	}

	if conf.Workers <= 0 {
		conf.Workers = 1
		logger.Info("[LOGREPLAY-OUTPUT] workers default value : ", conf.Workers)
	}

	if conf.CacheSize <= 0 {
		conf.CacheSize = cacheSizeMin // cache 缓存大小，单位 M
		logger.Info("[LOGREPLAY-OUTPUT] cache-size default value : ", conf.CacheSize)
	}

	if conf.RecordLimit <= 0 {
		conf.RecordLimit = recordLimit
		logger.Info("[LOGREPLAY-OUTPUT] record-limit default value : ", conf.RecordLimit)
	}

	if conf.QPSLimit <= 0 {
		conf.QPSLimit = defaultQPSLimit
		logger.Info("[LOGREPLAY-OUTPUT] qps-limit default value : ", conf.QPSLimit)
	}
}

func (o *LogReplayOutput) createTask() (uint32, error) {
	addMap := map[string][]string{
		"default_target": o.targetList,
	}

	data, _ := json.Marshal(addMap)
	req := &logreplay.GoReplayReq{
		ModuleID:       o.conf.ModuleID,
		Operator:       goreplay,
		Total:          uint32(o.conf.RecordLimit),
		Rate:           100,
		RecordCommitID: goreplay,
		TargetModuleID: o.conf.ModuleID,
		Addrs:          string(data),
	}

	// 表示双环境回放
	if len(o.targetList) == 2 {
		req.ReplayEnvTag = true
	}

	rsp := &logreplay.TaskRsp{}

	err := gateway.Send(config.GWCfg().GoReplayTaskURL, req, rsp)
	if err != nil {
		logger.Fatal("[LOGREPLAY-OUTPUT] createTask error, get taskID failed: ", err)
		return 0, err
	}

	return rsp.TaskID, nil
}

func (o *LogReplayOutput) startWorker(bufferIndex int) {
	for {
		if o.counter.RecordNum() > uint32(o.conf.RecordLimit) {
			logger.Fatal("[LOGREPLAY-OUTPUT] already access record max limit: ", o.conf.RecordLimit)
			return
		}
		msg := <-o.buf[bufferIndex]

		if !protocol.IsOriginPayload(msg.Meta) {
			return
		}

		o.sendMsgLogReplay(msg)
	}
}

func (o *LogReplayOutput) startReporter() {
	logreplay.NewReporter(o.counter, o.reportBuf).Run()
}

func (o *LogReplayOutput) isQPSOver() bool {
	now := time.Now().UnixNano()
	if (time.Now().UnixNano() - atomic.LoadInt64(&o.lastSampleTime)) > time.Second.Nanoseconds() {
		atomic.StoreInt64(&o.lastSampleTime, now)
		atomic.StoreUint32(&o.curQPS, 0)
	} else {
		atomic.AddUint32(&o.curQPS, 1)
	}

	return atomic.LoadUint32(&o.curQPS) > uint32(o.conf.QPSLimit)
}

func (o *LogReplayOutput) getBufferIndex(data []byte) int {
	hasher := fnv.New32a()
	_, _ = hasher.Write(data)

	return int(hasher.Sum32()) % o.conf.Workers
}

func getReqKey(uuid string) []byte {
	return []byte(fmt.Sprintf(goReplayRequestKey, uuid))
}

func getReqHeaderKey(uuid string) []byte {
	return []byte(fmt.Sprintf(goReplayReqHeaderKey, uuid))
}

func getReqRspKey(uuid string) []byte {
	return []byte(fmt.Sprintf(goReplayReplayRespKey, uuid))
}

func (o *LogReplayOutput) sendMsgLogReplay(msg *Message) {
	uuid := protocol.PayloadID(msg.Meta)
	uuidStr := string(uuid)

	logger.Debug3(fmt.Sprintf("[LOGREPLAY-OUTPUT] sendMsgLogReplay msg meta: %s, uuidStr: %s, protocol:%s",
		byteutils.SliceToString(msg.Meta), uuidStr, msg.Protocol))
	// 如果拦截的是res payload, 但是没有缓存req，直接返回
	cacheReq, _ := o.cache.Get(getReqKey(uuidStr))
	if msg.Meta[0] == protocol.ResponsePayload && len(cacheReq) == 0 {
		logger.Debug3("[LOGREPLAY-OUTPUT] sendMsgLogReplay, discard rsp uuid ", uuidStr)
		return
	}

	// 解析 request 包
	if msg.Meta[0] == protocol.RequestPayload {
		headerCodec := codec.GetHeaderCodec(msg.Protocol)
		o.parseReq(msg, headerCodec, uuidStr)
		o.setFullReqMsgCache(uuidStr, msg)
	} else if msg.Meta[0] == protocol.ResponsePayload { // 解析 response 包
		logger.Debug3("[LOGREPLAY-OUTPUT] sendMsgLogReplay, start parse rsp, uuid: ", uuidStr)
		record, err := o.parseResponse(msg, cacheReq, uuidStr)
		if err != nil {
			logger.Info("[LOGREPLAY-OUTPUT] parseResponse err: ", err)
			return
		}

		// 录制数据上报 LogReplay
		o.sendRequest(uuidStr, record)
		param := replayParam{
			uuidStr: uuidStr, traceID: record.TraceID,
		}
		o.replayBuf <- param
	}
}

func (o *LogReplayOutput) doReplay(msg *Message, header codec.ProtocolHeader, target string) []byte {
	if !protocol.IsRequestPayload(msg.Meta) {
		return nil
	}

	if msg.Protocol == codec.GrpcName && o.conf.GrpcReplayMethodName != "" &&
		!strings.Contains(o.conf.GrpcReplayMethodName, header.MethodName) {
		logger.Debug3("[LOGREPLAY-OUTPUT]  grpc protocol method not match, header method: ",
			header.MethodName, ",config interface name: "+o.conf.GrpcReplayMethodName, ",")

		return nil
	}
	return nil
}

func (o *LogReplayOutput) parseResponse(msg *Message, cacheReq []byte,
	uuidStr string) (*logreplay.GoReplayMessage, error) {
	cacheHeaderBytes, err := o.cache.Get(getReqHeaderKey(uuidStr))
	if err != nil {
		return nil, err
	}

	var cacheReqHeader codec.ProtocolHeader
	if err = o.json.Unmarshal(cacheHeaderBytes, &cacheReqHeader); err != nil {
		return nil, err
	}

	// traceID := tracer.NewTraceIDWithParent("").String()
	// TODO 生成 traceID
	traceID := uuidStr
	if cacheReqHeader.CusTraceID != "" {
		traceID = cacheReqHeader.CusTraceID
	}

	data := &logreplay.GoReplayMessage{
		RecordMessage: logreplay.RecordMessage{
			ModuleID:      o.conf.ModuleID,
			CommitID:      o.conf.CommitID,
			Time:          float64(time.Now().UnixNano()),
			TraceID:       traceID,
			InstanceName:  o.address,
			ServiceName:   cacheReqHeader.ServiceName,
			APIName:       cacheReqHeader.APIName,
			Protocol:      msg.Protocol,
			Src:           goreplay,
			ResponseBytes: msg.Data,
		},
		ProtocolServiceName: o.conf.ProtocolServiceName,
		MethodName:          cacheReqHeader.MethodName,
		InterfaceName:       cacheReqHeader.InterfaceName,
	}

	tag := make(map[string]interface{})
	tag["isGoReplay"] = trueStr
	tag["realServerName"] = o.conf.RealServerName
	tag["serverAddr"] = strings.Join(config.Settings.InputRAW, ";")
	tag["clientAddr"] = msg.SrcAddr
	if o.conf.LogreplayTaskID != "" {
		tag["goreplayTaskID"] = o.conf.LogreplayTaskID
	}
	if len(o.conf.RecordTagInfo) > 0 {
		tags := strings.Split(o.conf.RecordTagInfo, ";")
		for _, inputTag := range tags {
			sp := strings.Split(inputTag, ":")
			if len(sp) != 2 {
				continue
			}
			tag[sp[0]] = sp[1]
		}
	}

	data.RequestBytes = o.appendAfterClientPreface(msg.Protocol, cacheReq)
	data.Tag = tag

	return data, nil
}

func (o *LogReplayOutput) appendAfterClientPreface(protocol string, src []byte) []byte {
	if protocol != codec.GrpcName {
		return src
	}

	esf, _ := hex.DecodeString(emptySettingFrame)
	ret := append([]byte(http2.ClientPreface), esf...)

	return append(ret, src...)
}

func (o *LogReplayOutput) parseReq(msg *Message, headerCodec codec.HeaderCodec, uuidStr string) {
	// 先解析请求头,注意这里解析的请求的 data
	ph, err := headerCodec.Decode(msg.Data, msg.ConnectionID)
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] decode request header error: ", err)
		return
	}

	if o.isQPSOver() {
		logger.Warn("[LOGREPLAY-OUTPUT] already access record qps limit: ", atomic.LoadUint32(&o.curQPS),
			o.conf.QPSLimit)
		return
	}

	logger.Debug3(fmt.Sprintf("[LOGREPLAY-OUTPUT] %s req cache Key: %s", uuidStr, getReqKey(uuidStr)))
	// 缓存请求数据
	err = o.cache.Set(getReqKey(uuidStr), msg.Data, freeCacheExpired)
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] cache req err: ", err)
		return
	}

	phBytes, err := o.json.Marshal(ph)
	if err != nil {
		logger.Info("[LOGREPLAY-OUTPUT] marshal req header err: ", err)
		return
	}
	logger.Debug3("[LOGREPLAY-OUTPUT] record header", string(phBytes))
	// 缓存请求的 header
	err = o.cache.Set(getReqHeaderKey(uuidStr), phBytes, freeCacheExpired)
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] cache rea header err: ", err)
		return
	}
}

func (o *LogReplayOutput) sendRequest(uuidStr string, record *logreplay.GoReplayMessage) {
	// 生成回放任务
	if o.conf.Target != "" && o.taskID == 0 {
		o.setTaskID(uuidStr)
	}

	record.TaskID = o.taskID
	record.Success = atomic.LoadUint32(&o.success)
	record.DialFailed = atomic.LoadUint32(&o.dialFail)
	record.WriteFailed = atomic.LoadUint32(&o.writeFail)
	record.ReadFailed = atomic.LoadUint32(&o.readFail)
	record.SendFailed = record.DialFailed + record.WriteFailed + record.ReadFailed

	recordStr, marErr := o.json.Marshal(record)
	if marErr != nil {
		logger.Error("[LOGREPLAY-OUTPUT] sendRequest error, Marshal record failed: ",
			uuidStr, marErr)

		return
	}

	item := logreplay.ReportItem{Type: reportType, Data: string(recordStr)}

	o.reportBuf <- item
}

// PluginWrite writes message to this plugin
func (o *LogReplayOutput) PluginWrite(msg *Message) (n int, err error) {
	if !protocol.IsOriginPayload(msg.Meta) {
		return len(msg.Data), nil
	}

	uuid := protocol.PayloadID(msg.Meta)
	bufferIndex := o.getBufferIndex(uuid)
	o.buf[bufferIndex] <- msg

	return len(msg.Data) + len(msg.Meta), nil
}

// PluginRead reads message from this plugin
func (o *LogReplayOutput) PluginRead() (*Message, error) {
	if !o.conf.TrackResponses {
		return nil, errors.ErrorStopped
	}

	var (
		resp *response
		msg  Message
	)

	select {
	case <-o.stop:
		return nil, errors.ErrorStopped
	case resp = <-o.responses:
		msg.Data = resp.payload
	}

	msg.Meta = protocol.PayloadHeader(protocol.ReplayedResponsePayload, resp.uuid,
		resp.roundTripTime, resp.startedAt)

	return &msg, nil
}

// String string logreplay output
func (o *LogReplayOutput) String() string {
	return fmt.Sprintf("LogReplay output: %s", o.conf.ModuleID)
}

// Close closes the data channel so that data
func (o *LogReplayOutput) Close() error {
	close(o.stop)
	return nil
}

func checkAddress(address string) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		logger.Fatal("[LOGREPLAY-OUTPUT] parsing address error: ", err)
		return
	}

	if host == "" || port == "" || net.ParseIP(host) == nil {
		logger.Fatal("[LOGREPLAY-OUTPUT] invalid address: ", address, err)
		return
	}
}

// getEnvInfo 获取环境名称
func getEnvInfo() (string, error) {
	return os.Hostname()
}
