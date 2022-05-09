package config

import (
	"gopkg.in/yaml.v2"
)

// GWConfig Config 后台网关的配置
type GWConfig struct {
	Host                        string `yaml:"host"`
	ReportURL                   string `yaml:"report_url"`
	GoReplayTaskURL             string `yaml:"go_replay_task_url"`
	GetGoReplayTaskStatus       string `yaml:"get_go_replay_task_status"`
	UpdateGoReplayTaskRecordNum string `yaml:"update_go_replay_task_record_num"`
}

var cfg GWConfig

// SetUp 初始化
func SetUp(yamlConfig []byte) error {
	if err := yaml.Unmarshal(yamlConfig, &cfg); err != nil {
		return err
	}

	return nil
}

// GWCfg 读取网关配置
func GWCfg() GWConfig {
	return cfg
}
