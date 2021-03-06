package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	logging "github.com/op/go-logging"
	"github.com/shadowsocks/go-shadowsocks2/core"
)

var (
	logger     *logging.Logger
	logLevel   = logging.DEBUG
	udpTimeout time.Duration
)

const (
	logPath = "./sockstun.log"
	pidName = "sockstun.pid"
)

func setLogger(level logging.Level) {
	file, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666) //打开文件
	if err != nil {
		fmt.Println("init log failed", err)
		panic(0)
	}
	format := logging.MustStringFormatter(
		`%{color}%{time:2006-01-02 15:04:05.000} %{shortfile} %{shortfunc} %{level:.4s} %{color:reset} %{message}`,
	)
	backend := logging.NewLogBackend(file, "", 0)
	backendFormatter := logging.NewBackendFormatter(backend, format)
	backendLeveled := logging.AddModuleLevel(backendFormatter)
	backendLeveled.SetLevel(level, "")
	logger = logging.MustGetLogger("main")
	logger.SetBackend(backendLeveled)
}

func writePID(fileName string) (err error) {
	pid := os.Getpid()
	f, err := os.Create(fileName)
	if err != nil {
		return
	}
	defer f.Close()
	_, err = f.WriteString(fmt.Sprintf("%d\n", pid))
	f.Sync()
	return
}

func main() {
	configPath := flag.String("c", "config.json", "your config file")
	flag.Parse()

	conf, err := newConfig(*configPath)
	if err != nil {
		return
	}
	udpTimeout = conf.UDPTimeout

	if conf.LogLevel >= logging.ERROR && conf.LogLevel <= logging.DEBUG {
		logLevel = conf.LogLevel
	}
	setLogger(logLevel)

	if _, err := newInfluxdbClient(conf.InfluxDB); err != nil {
		return
	}

	if err = writePID(pidName); err != nil {
		logger.Error("write pid failed: %v", err)
		return
	}

	for _, node := range conf.Nodes {
		ciph, err := core.PickCipher(node.Cipher, nil, node.Password)
		if err != nil {
			logger.Error("PickCipher err:%v", err)
			return
		}
		go udpRemote(conf.Server, node.Port, ciph.PacketConn)
		go tcpRemote(conf.Server, node.Port, ciph.StreamConn)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	return
}
