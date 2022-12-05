// Gor is simple http traffic replication tool written in Go.
// Its main goal to replay traffic from production servers to staging and dev environments.
// Now you can test your code on real user sessions in an automated and repeatable fashion.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"goreplay/config"
	"goreplay/emitter"
	"goreplay/logger"
	"goreplay/plugins"

	"github.com/shirou/gopsutil/process"

	_ "embed"
	_ "go.uber.org/automaxprocs"
	_ "goreplay/tcp/protocol"
)

// 常量
const (
	psName             = "goreplay" // 设置 goReplay 的进程名
	goreplayServerName = "goreplay_server"
)

//go:embed gateway.yml
var yamlConfig []byte

func setupYamlConf() {
	if err := config.SetUp(yamlConfig); err != nil {
		logger.Fatal(err)
	}
	logger.Info(fmt.Sprintf("gw config: %+v", config.GWCfg()))
}

func main() {
	args := os.Args[1:]
	var inOutPlugins *plugins.InOutPlugins
	if len(args) > 0 && args[0] == "file-server" {
		if len(args) != 2 {
			logger.Fatal("You should specify port and IP (optional) for the file server. Example: `gor file-server :80`")
		}
		dir, _ := os.Getwd()

		logger.Info("Started example file server for current directory on address ", args[1])

		logger.Fatal(http.ListenAndServe(args[1], loggingMiddleware(http.FileServer(http.Dir(dir)))))
	} else {
		flag.Parse()
		setupYamlConf() // it must be after flag.Parse(), coz it may be overwritten by flags
		if config.Settings.LogPath != "" {
			logger.Info("log output path: ", config.Settings.LogPath)
		}

		if config.Settings.OnlyOneProcess {
			limitProcess()
		}

		inOutPlugins = plugins.NewPlugins(plugins.InitPluginSettings())
	}

	logger.Info(fmt.Sprintf("[PPID %d and PID %d] Version:%s", os.Getppid(), os.Getpid(), config.VERSION))

	if inOutPlugins == nil {
		logger.Fatal("inOutPlugins is nil")
		return
	}

	if len(inOutPlugins.Inputs) == 0 || len(inOutPlugins.Outputs) == 0 {
		logger.Fatal("Required at least 1 input and 1 output")
	}
	closeCh := make(chan int)
	emitterSettings := emitter.Settings{
		PrettifyHTTP:   config.Settings.PrettifyHTTP,
		CopyBufferSize: config.Settings.CopyBufferSize,
		Split:          config.Settings.SplitOutput,
		ModifierConfig: config.Settings.ModifierConfig,
	}
	emitter := emitter.NewEmitter(emitterSettings)
	go emitter.Start(inOutPlugins, config.Settings.Middleware)
	goExitAfter(closeCh)

	c := make(chan os.Signal, 1)
	// signal.Notify(c, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM,
	// syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
	signal.Notify(c, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM,
		syscall.SIGQUIT)
	exit := 0
	select {
	case sg := <-c:
		logger.Info(fmt.Sprintf("goreplay exit by signal %s", sg.String()))
		exit = 1
	case <-closeCh:
		exit = 0
	}

	emitter.Close()
	os.Exit(exit)
}

// goExitAfter 预处理 exit_after 时间到之后的退出逻辑。
func goExitAfter(closeCh chan int) {
	// exit_after == 0, 表示不需要时间限制，一直录制
	if config.Settings.ExitAfter == 0 {
		return
	}
	// exit_after == -1：表示外部没有进行设置,应该初始化成默认值。
	if config.Settings.ExitAfter == -1 {
		// 设置默认时间为 60min。
		config.Settings.ExitAfter = 6 * time.Hour
		time.AfterFunc(config.Settings.ExitAfter/2, func() {
			logger.Info(fmt.Sprintf("gor run timeout is half,time: %s\n", config.Settings.ExitAfter/2))
			// 用户未使用 logreplay 进行流量收集的情况。
			if config.Settings.OutputLogReplayConfig.ModuleID == "" {
				logger.Debug2("goreplay run no output logreplay")
				return
			}
			ExitProccess(config.Settings.ExitAfter / 2)
		})
	}

	time.AfterFunc(config.Settings.ExitAfter, func() {
		logger.Info("gor run timeout %s\n", config.Settings.ExitAfter)
		if config.Settings.OutputLogReplayConfig.ModuleID == "" {
			logger.Debug2("goreplay run output logreplay with empty moduleID")
		} else {
			ExitProccess(config.Settings.ExitAfter)
		}
		close(closeCh)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rb, _ := httputil.DumpRequest(r, false)
		logger.Info(string(rb))
		next.ServeHTTP(w, r)
	})
}

func limitProcess() {
	processList, err := process.Processes()
	if err != nil {
		logger.Fatal(err)
	}

	for _, p := range processList {
		pn, err := p.Name()
		if err != nil {
			logger.Fatal(err)
		}
		if strings.Contains(pn, psName) && int(p.Pid) != os.Getpid() {
			// 排除 goreplay_server 进程引起的误识别
			if pn == goreplayServerName {
				continue
			}
			logger.Fatal(fmt.Errorf("process %s exists, pid=%d", psName, p.Pid))
		}
	}
}
