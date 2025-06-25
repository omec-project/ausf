// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2024 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/omec-project/ausf/callback"
	"github.com/omec-project/ausf/consumer"
	"github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/ausf/metrics"
	"github.com/omec-project/ausf/nrfregistration"
	"github.com/omec-project/ausf/polling"
	"github.com/omec-project/ausf/ueauthentication"
	nrfCache "github.com/omec-project/openapi/nrfcache"
	"github.com/omec-project/util/http2_util"
	utilLogger "github.com/omec-project/util/logger"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type AUSF struct{}

type (
	// Config information.
	Config struct {
		cfg string
	}
)

var config Config

var ausfCLi = []cli.Flag{
	&cli.StringFlag{
		Name:     "cfg",
		Usage:    "ausf config file",
		Required: true,
	},
}

func (*AUSF) GetCliCmd() (flags []cli.Flag) {
	return ausfCLi
}

func (ausf *AUSF) Initialize(c *cli.Command) error {
	config = Config{
		cfg: c.String("cfg"),
	}

	absPath, err := filepath.Abs(config.cfg)
	if err != nil {
		logger.CfgLog.Errorln(err)
		return err
	}

	if err := factory.InitConfigFactory(absPath); err != nil {
		return err
	}

	ausf.setLogLevel()

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	factory.AusfConfig.CfgLocation = absPath
	context.Init()
	go polling.PollNetworkConfig()

	return nil
}

func (ausf *AUSF) setLogLevel() {
	if factory.AusfConfig.Logger == nil {
		logger.InitLog.Warnln("AUSF config without log level setting")
		return
	}

	if factory.AusfConfig.Logger.AUSF != nil {
		if factory.AusfConfig.Logger.AUSF.DebugLevel != "" {
			if level, err := zapcore.ParseLevel(factory.AusfConfig.Logger.AUSF.DebugLevel); err != nil {
				logger.InitLog.Warnf("AUSF Log level [%s] is invalid, set to [info] level",
					factory.AusfConfig.Logger.AUSF.DebugLevel)
				logger.SetLogLevel(zap.InfoLevel)
			} else {
				logger.InitLog.Infof("AUSF Log level is set to [%s] level", level)
				logger.SetLogLevel(level)
			}
		} else {
			logger.InitLog.Warnln("AUSF Log level not set. Default set to [info] level")
			logger.SetLogLevel(zap.InfoLevel)
		}
	}
}

func (ausf *AUSF) FilterCli(c *cli.Command) (args []string) {
	for _, flag := range ausf.GetCliCmd() {
		name := flag.Names()[0]
		value := fmt.Sprint(c.Generic(name))
		if value == "" {
			continue
		}

		args = append(args, "--"+name, value)
	}
	return args
}

func (ausf *AUSF) Start() {
	logger.InitLog.Infoln("server started")

	router := utilLogger.NewGinWithZap(logger.GinLog)
	ueauthentication.AddService(router)
	callback.AddService(router)

	go metrics.InitMetrics()

	self := context.GetSelf()

	addr := fmt.Sprintf("%s:%d", self.BindingIPv4, self.SBIPort)

	if self.EnableNrfCaching {
		logger.InitLog.Infoln("enable NRF caching feature")
		nrfCache.InitNrfCaching(self.NrfCacheEvictionInterval*time.Second, consumer.SendNfDiscoveryToNrf)
	}

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		ausf.Terminate()
		os.Exit(0)
	}()

	sslLog := filepath.Dir(factory.AusfConfig.CfgLocation) + "/sslkey.log"
	server, err := http2_util.NewServer(addr, sslLog, router)
	if server == nil {
		logger.InitLog.Errorf("initialize HTTP server failed: %v", err)
		return
	}

	if err != nil {
		logger.InitLog.Warnf("initialize HTTP server: %v", err)
	}

	serverScheme := factory.AusfConfig.Configuration.Sbi.Scheme
	switch serverScheme {
	case "http":
		err = server.ListenAndServe()
	case "https":
		err = server.ListenAndServeTLS(self.PEM, self.Key)
	default:
		logger.InitLog.Fatalf("HTTP server setup failed: invalid server scheme %+v", serverScheme)
		return
	}

	if err != nil {
		logger.InitLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (ausf *AUSF) Exec(c *cli.Command) error {
	logger.InitLog.Debugln("args:", c.String("cfg"))
	args := ausf.FilterCli(c)
	logger.InitLog.Debugln("filter:", args)
	command := exec.Command("ausf", args...)

	stdout, err := command.StdoutPipe()
	if err != nil {
		logger.InitLog.Fatalln(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			logger.InitLog.Infoln(in.Text())
		}
		wg.Done()
	}()

	stderr, err := command.StderrPipe()
	if err != nil {
		logger.InitLog.Fatalln(err)
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			logger.InitLog.Infoln(in.Text())
		}
		wg.Done()
	}()

	go func() {
		startErr := command.Start()
		if startErr != nil {
			logger.InitLog.Fatalln(startErr)
		}
		wg.Done()
	}()

	wg.Wait()

	return err
}

func (ausf *AUSF) Terminate() {
	logger.InitLog.Infof("terminating AUSF")
	nrfregistration.DeregisterNF()
	logger.InitLog.Infoln("AUSF terminated")
}
