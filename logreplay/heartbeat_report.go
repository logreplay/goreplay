package logreplay

import (
	"time"

	"goreplay/config"
	"goreplay/logger"
	"goreplay/utils/gateway"
)

// HeartBeatReporter 心跳上报
type HeartBeatReporter struct {
	LogreplayTaskID string
}

// DoReport 执行心跳上报
func (h *HeartBeatReporter) DoReport() {
	logger.Debug("report heartbeat...")

	ipAndPort := getReportAddress()
	logger.Debug("ip and port:", ipAndPort)
	if ipAndPort == "" {
		return
	}
	req := &GetGoReplayTaskStatusReq{
		TaskID: h.LogreplayTaskID,
	}

	rsp := &GetGoReplayTaskStatusRsp{}

	// 调用 goreplay_server 上报心跳
	if err := gateway.Send(config.GWCfg().GetGoReplayTaskStatus, req, rsp); err != nil {
		logger.Error(err)
	}

	// 定时上报
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		err := gateway.Send(config.GWCfg().GetGoReplayTaskStatus, req, rsp)
		if err != nil {
			logger.Error(err)
		}
		if rsp.Status == TaskStatusStop {
			logger.Fatal("goreplay process stoped")
		}
	}
}

// getReportAddress 获取上报的地址
func getReportAddress() string {
	if len(config.Settings.InputRAW) > 0 {
		return config.Settings.InputRAW[0]
	}

	if len(config.Settings.InputUDP) > 0 {
		return config.Settings.InputUDP[0]
	}
	return ""
}
