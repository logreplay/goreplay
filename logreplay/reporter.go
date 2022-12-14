// Package logreplay 上报逻辑
package logreplay

import (
	"sync"
	"time"

	"goreplay/config"
	"goreplay/logger"
	"goreplay/utils/gateway"
)

// Reporter reports the data to logreplay
type Reporter struct {
	items     []ReportItem
	timer     *time.Ticker
	reportBuf chan ReportItem
	lock      sync.Mutex
	counter   *Counter
}

// NewReporter new reporter
func NewReporter(counter *Counter, reportBuf chan ReportItem) *Reporter {
	return &Reporter{
		items:     []ReportItem{},
		timer:     time.NewTicker(3 * time.Second),
		reportBuf: reportBuf,
		lock:      sync.Mutex{},
		counter:   counter,
	}
}

// Run reporter开始运行
func (r *Reporter) Run() {
	var (
		stop bool
	)

	for !stop {
		select {
		case item, isOpen := <-r.reportBuf:
			if isOpen {
				r.items = append(r.items, item)
				// commit every 100
				if len(r.items) > 100 {
					r.commit()
				}
			} else {
				r.timer.Stop()
				stop = true
				r.commit()

			}
		case <-r.timer.C:
			// commit in fix time
			r.commit()
		}
	}
}

// commit patch
func (r *Reporter) commit() {
	r.lock.Lock()
	defer r.lock.Unlock()
	reqs := r.items

	r.report(reqs)
	r.items = make([]ReportItem, 0)
}

// do report
func (r *Reporter) report(items []ReportItem) {
	if len(items) > 0 {
		rsp := &ReportResp{}
		if err := gateway.Send(config.GWCfg().ReportURL, &ReportData{Batch: items}, rsp); err != nil {
			logger.Warn("[LOGREPLAY-OUTPUT] report LogReplay error: ", err)
			return
		}

		if rsp.BaseResp.Code != HttpRspSuccCode {
			logger.Error("[LOGREPLAY-OUTPUT] report failed, errCode:%d, errMsg:%s", rsp.BaseResp.Code, rsp.BaseResp.Msg)
		} 

		logger.Info("[LOGREPLAY-OUTPUT] total number of reported records: ", r.counter.IncrRecordNum(uint32(rsp.Succeed)))
		logger.Debug2("[LOGREPLAY-OUTPUT] report rsp succeed: ", rsp.Succeed)
	}
}
