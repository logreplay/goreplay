// Package plugins 各种插件实现
package plugins

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/panjf2000/ants/v2"

	"goreplay/codec"
	"goreplay/config"
	"goreplay/logger"
	"goreplay/logreplay"
	"goreplay/utils/gateway"
)

const (
	goReplayFullRequestKey   = "full_req_%s"
	replayWorker             = 10
	baselineEnvResponseBytes = "BaselineEnvResponseBytes"
	testedEvnResponseBytes   = "TestedEvnResponseBytes"
)

// param for replaying
type replayParam struct {
	uuidStr string
	traceID string
}

// key for full req
func getFullReqKey(uuid string) []byte {
	return []byte(fmt.Sprintf(goReplayFullRequestKey, uuid))
}

// cache the req
func (o *LogReplayOutput) setFullReqMsgCache(uuid string, msg *Message) {
	msgByte, err := json.Marshal(msg)
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] setFullReqMsgCache Marshal msg error:", err)
		return
	}
	err = o.cache.Set(getFullReqKey(uuid), msgByte, freeCacheExpired)
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] setFullReqMsgCache Set error:", err)
	}
}

// send to the target ip
func (o *LogReplayOutput) sendReplayRequest(uuidStr string, traceID string) {
	if len(o.targetList) == 0 {
		return
	}
	// clear
	defer func() {
		o.cache.Del(getReqKey(uuidStr))
		o.cache.Del(getReqRspKey(uuidStr))
		o.cache.Del(getReqHeaderKey(uuidStr))
		o.cache.Del(getFullReqKey(uuidStr))
	}()

	// 获取数据header
	phBytes, err := o.cache.Get(getReqHeaderKey(uuidStr))
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] sendReplayRequest get phBytes error: ", err)
		return
	}
	var ph codec.ProtocolHeader
	// unmarshal header
	err = o.json.Unmarshal(phBytes, &ph)
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] sendReplayRequest Unmarshal header error: ", err)
		return
	}
	// 请求回放接口
	msgByte, err := o.cache.Get(getFullReqKey(uuidStr))
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] sendReplayRequest get msg error:", err)
		return
	}
	var msg Message
	// unmarshal message
	err = json.Unmarshal(msgByte, &msg)
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] sendReplayRequest Unmarshal msg error:", err)
		return
	}

	if !o.isAllowedRequest(&msg) {
		return
	}

	requests := []*Message{&msg}
	if len(o.targetList) == 2 {
		// replay to two env
		duplicateRequest := msg
		requests = append(requests, &duplicateRequest)
	}

	o.modifyRequest(requests)
	// response data for replaying
	response := o.getReplayData(requests, ph)
	if response == nil {
		return
	}

	if o.taskID == 0 {
		o.setTaskID(uuidStr)
	}
	record := logreplay.ReplayMessage{
		ModuleID:   o.conf.ModuleID,
		CommitID:   o.conf.CommitID,
		TaskID:     o.taskID,
		TraceID:    traceID,
		Time:       float64(time.Now().UnixNano()),
		Response:   response,
		Protocol:   msg.Protocol,
		ReplayType: 0,
	}
	// 双环境回放标示
	if len(o.targetList) == 2 {
		record.ReplayType = logreplay.TwoEnvReplayType
	}

	recordStr, marErr := o.json.Marshal(record)
	if marErr != nil {
		logger.Error("[LOGREPLAY-OUTPUT] sendReplayRequest error, Marshal record failed: ",
			uuidStr, marErr)

		return
	}

	item := logreplay.ReportItem{Type: replayType, Data: string(recordStr)}

	o.reportBuf <- item
}

// getReplayData 获取回放数据，适配单环境/双环境
func (o *LogReplayOutput) getReplayData(requests []*Message, ph codec.ProtocolHeader) map[string]interface{} {
	if len(o.targetList) == 2 {
		baselineEnvResponse := o.doReplay(requests[0], ph, o.targetList[0])
		testedEvnResponse := o.doReplay(requests[1], ph, o.targetList[1])
		if baselineEnvResponse == nil || testedEvnResponse == nil {
			return nil
		}

		return map[string]interface{}{
			baselineEnvResponseBytes: baselineEnvResponse,
			testedEvnResponseBytes:   testedEvnResponse,
		}
	}
	// 正常回放
	baselineEnvResponse := o.doReplay(requests[0], ph, o.target)
	return map[string]interface{}{
		"ReplayBytes": baselineEnvResponse,
	}
}

// is the request is allowed to replay
func (o *LogReplayOutput) isAllowedRequest(request *Message) bool {
	// Only replay allowed request
	if len(o.conf.AllowRequest) == 0 {
		return true
	}
	for _, f := range o.conf.AllowRequest {
		if f.Regexp.Match(request.Data) {
			return true
		}
	}
	logger.Debug2("[LOGREPLAY-OUTPUT] do not match allow-request regular expression, do NOT replay")
	return false
}

// modifyRequest before replay
func (o *LogReplayOutput) modifyRequest(requests []*Message) {
	if len(o.conf.RequestReWrite) == 0 {
		return
	}

	for i, f := range o.conf.RequestReWrite {
		// rewrite request if match
		if f.Regexp.Match(requests[i].Data) {
			logger.Debug2("[LOGREPLAY-OUTPUT] before the replacement, the request data is ", string(requests[i].Data))
			requests[i].Data = f.Regexp.ReplaceAll(requests[i].Data, f.Target)
			logger.Debug2("[LOGREPLAY-OUTPUT] after the replacement, the request data is ", string(requests[i].Data))
		}
	}
}

// set the taskID
func (o *LogReplayOutput) setTaskID(uuidStr string) {
	taskID, err := o.createTask()
	if err != nil {
		logger.Debug2("[LOGREPLAY-OUTPUT] sendRequest, createTask failed uuid ", uuidStr)
		return
	}

	o.taskID = taskID
}

// begin to replay
func (o *LogReplayOutput) startRelay() {
	pool, err := ants.NewPool(replayWorker)
	if err != nil {
		logger.Error("[LOGREPLAY-OUTPUT] startRelay error:  ", err)
		return
	}
	for {
		param := <-o.replayBuf
		err = pool.Submit(func() {
			o.sendReplayRequest(param.uuidStr, param.traceID)
		})
		if err != nil {
			logger.Error("[LOGREPLAY-OUTPUT] Submit error:  ", err)
		}
	}
}

func (o *LogReplayOutput) doReport() {
	heartBeat := logreplay.HeartBeatReporter{
		LogreplayTaskID: o.conf.LogreplayTaskID,
	}
	// report heartbeat
	go heartBeat.DoReport()
	// report record num
	go o.reportRecordNum()
}

// report count of total recording
func (o *LogReplayOutput) reportRecordNum() {
	// taskID is require
	if o.conf.LogreplayTaskID == "" {
		return
	}
	// 定时1分钟上报更新一次
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		req := &logreplay.UpdateGoReplayTaskRecordNumReq{
			TaskID:    o.conf.LogreplayTaskID,
			RecordNum: int32(o.counter.RecordNum()),
		}
		rsp := &logreplay.UpdateGoReplayTaskRecordNumRsp{}
		if err := gateway.Send(config.GWCfg().UpdateGoReplayTaskRecordNum, req, rsp); err != nil {
			logger.Error("[LOGREPLAY-OUTPUT] reportRecordNum error: ", err)
		}
	}
}
