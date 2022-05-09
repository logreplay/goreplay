package plugins

import (
	"reflect"
	"strings"

	"goreplay/config"
)

// Settings plugins` settings
type Settings struct {
	InputTCP        config.MultiOption `json:"input-tcp"`
	InputTCPConfig  config.TCPInputConfig
	OutputTCP       config.MultiOption `json:"output-tcp"`
	OutputTCPConfig config.TCPOutputConfig

	InputFile        config.MultiOption `json:"input-file"`
	InputFileLoop    bool               `json:"input-file-loop"`
	OutputFile       config.MultiOption `json:"output-file"`
	OutputFileConfig config.FileOutputConfig

	InputRAW config.MultiOption `json:"input_raw"`
	config.RAWInputConfig

	OutputHTTP      config.MultiOption `json:"output-http"`
	OutputLogReplay bool               `json:"output-logreplay"`

	OutputHTTPConfig      config.HTTPOutputConfig
	OutputLogReplayConfig config.LogReplayOutputConfig

	OutputBinary       config.MultiOption `json:"output-binary"`
	OutputBinaryConfig config.BinaryOutputConfig

	ModifierConfig config.HTTPModifierConfig

	InputUDP       config.MultiOption `json:"input-udp"`
	InputUDPConfig config.UDPInputConfig

	InputKafkaConfig config.InputKafkaConfig
	KafkaTLSConfig   config.KafkaTLSConfig
}

// Message represents data accross plugins
type Message struct {
	Meta         []byte // metadata
	Data         []byte // actual data
	ConnectionID string
	SrcAddr      string // 记录来源的IP地址, 在包为response包的时候赋值, value为DstAddr
	Protocol     string
}

// PluginReader is an interface for input plugins
type PluginReader interface {
	PluginRead() (msg *Message, err error)
}

// PluginWriter is an interface for output plugins
type PluginWriter interface {
	PluginWrite(msg *Message) (n int, err error)
}

// PluginReadWriter is an interface for plugins that support reading and writing
type PluginReadWriter interface {
	PluginReader
	PluginWriter
}

// InOutPlugins struct for holding references to plugins
type InOutPlugins struct {
	Inputs  []PluginReader
	Outputs []PluginWriter
	All     []interface{}
}

// extractLimitOptions detects if plugin get called with limiter support
// Returns address and limit
func extractLimitOptions(options string) (string, string) {
	split := strings.Split(options, "|")

	if len(split) > 1 {
		return split[0], split[1]
	}

	return split[0], ""
}

// Automatically detects type of plugin and initialize it
//
// See this article if curious about reflect stuff below: http://blog.burntsushi.net/type-parametric-functions-golang
func (p *InOutPlugins) registerPlugin(constructor interface{}, options ...interface{}) {
	var path, limit string
	vc := reflect.ValueOf(constructor)

	// Pre-processing options to make it work with reflect
	var vo []reflect.Value
	for _, oi := range options {
		vo = append(vo, reflect.ValueOf(oi))
	}

	if len(vo) > 0 {
		// Removing limit options from path
		path, limit = extractLimitOptions(vo[0].String())

		// Writing value back without limiter "|" options
		vo[0] = reflect.ValueOf(path)
	}

	// Calling our constructor with list of given options
	plugin := vc.Call(vo)[0].Interface()

	if limit != "" {
		plugin = NewLimiter(plugin, limit)
	}

	// Some of the output can be Readers as well because return responses
	if r, ok := plugin.(PluginReader); ok {
		p.Inputs = append(p.Inputs, r)
	}

	if w, ok := plugin.(PluginWriter); ok {
		p.Outputs = append(p.Outputs, w)
	}

	p.All = append(p.All, plugin)

}

// InitPluginSettings 将公共参数转为plugins的私有参数
func InitPluginSettings() Settings {
	pluginSettings := Settings{
		InputTCP:              config.Settings.InputTCP,
		InputTCPConfig:        config.Settings.InputTCPConfig,
		OutputTCP:             config.Settings.OutputTCP,
		OutputTCPConfig:       config.Settings.OutputTCPConfig,
		InputFile:             config.Settings.InputFile,
		InputFileLoop:         config.Settings.InputFileLoop,
		OutputFile:            config.Settings.OutputFile,
		OutputFileConfig:      config.Settings.OutputFileConfig,
		InputRAW:              config.Settings.InputRAW,
		RAWInputConfig:        config.Settings.RAWInputConfig,
		OutputHTTP:            config.Settings.OutputHTTP,
		OutputLogReplay:       config.Settings.OutputLogReplay,
		OutputHTTPConfig:      config.Settings.OutputHTTPConfig,
		OutputLogReplayConfig: config.Settings.OutputLogReplayConfig,
		OutputBinary:          config.Settings.OutputBinary,
		OutputBinaryConfig:    config.Settings.OutputBinaryConfig,
		ModifierConfig:        config.Settings.ModifierConfig,
		InputUDP:              config.Settings.InputUDP,
		InputUDPConfig:        config.Settings.InputUDPConfig,
		InputKafkaConfig:      config.Settings.InputKafkaConfig,
		KafkaTLSConfig:        config.Settings.KafkaTLSConfig,
	}

	return pluginSettings
}

// NewPlugins specify and initialize all available plugins
func NewPlugins(settings Settings) *InOutPlugins {
	plugins := new(InOutPlugins)

	for _, options := range settings.InputRAW {
		plugins.registerPlugin(NewRAWInput, options, settings.RAWInputConfig)
	}

	for _, options := range settings.InputTCP {
		plugins.registerPlugin(NewTCPInput, options, &settings.InputTCPConfig)
	}

	for _, options := range settings.OutputTCP {
		plugins.registerPlugin(NewTCPOutput, options, &settings.OutputTCPConfig)
	}

	for _, options := range settings.InputFile {
		plugins.registerPlugin(NewFileInput, options, settings.InputFileLoop)
	}

	for _, path := range settings.OutputFile {
		plugins.registerPlugin(NewFileOutput, path, &settings.OutputFileConfig)
	}

	// If we explicitly set Host header http output should not rewrite it
	// Fix: https://github.com/buger/gor/issues/174
	checkOriginalHost(&settings)

	for _, options := range settings.OutputHTTP {
		plugins.registerPlugin(NewHTTPOutput, options, &settings.OutputHTTPConfig)
	}

	if settings.OutputLogReplay {
		settings.OutputLogReplayConfig.Protocol = getInputProtocol(settings)
		plugins.registerPlugin(NewLogReplayOutput, "", &settings.OutputLogReplayConfig)
	}

	for _, options := range settings.OutputBinary {
		plugins.registerPlugin(NewBinaryOutput, options, &settings.OutputBinaryConfig)
	}

	for _, options := range settings.InputUDP {
		plugins.registerPlugin(NewUDPInput, options, settings.InputUDPConfig)
	}

	if settings.InputKafkaConfig.Host != "" && settings.InputKafkaConfig.Topic != "" {
		plugins.registerPlugin(NewKafkaInput, "", &settings.InputKafkaConfig, &settings.KafkaTLSConfig)
	}

	return plugins
}

func checkOriginalHost(settings *Settings) {
	for _, header := range settings.ModifierConfig.Headers {
		if header.Name == "Host" {
			settings.OutputHTTPConfig.OriginalHost = true
			break
		}
	}
}

// getInputProtocol get input protocol
func getInputProtocol(settings Settings) string {
	if settings.Logreplay {
		if settings.InputKafkaConfig.Host != "" {
			return "kafka"
		}
		return settings.RAWInputConfig.Protocol
	}

	return settings.InputUDPConfig.Protocol
}
