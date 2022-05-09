// Package gateway  网关相关
package gateway

import (
	"fmt"
	"net/http"

	"goreplay/config"
	"goreplay/logger"

	"github.com/go-resty/resty/v2"
)

const schema = "http"

// http client
var cli = resty.New().
	SetHeader("Rewrite-Request", "true")

// Send 发送请求
func Send(url string, req, rsp interface{}) error {
	post, err := cli.
		SetBaseURL(fmt.Sprintf("%s://%s", schema, config.GWCfg().Host)).
		SetHeader("AppId", config.Settings.OutputLogReplayConfig.APPID).
		SetHeader("AppKey", config.Settings.OutputLogReplayConfig.APPKey).
		R().SetBody(req).SetResult(rsp).Post(url)
	// resp is nil
	if post == nil {
		return fmt.Errorf("%s: rsp is nil", url)
	}
	// request fail
	if post.StatusCode() != http.StatusOK {
		logger.Error(fmt.Sprintf("url: %s, httpStatus: %d, req: %v", url, post.StatusCode(), req))
	}
	// return err
	if err != nil {
		return fmt.Errorf("remote error: %v, rsp: %s", err, post)
	}
	return nil
}
