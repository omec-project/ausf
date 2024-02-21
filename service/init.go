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
	"sync"
	"syscall"
	"time"

	"github.com/omec-project/ausf/context"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/omec-project/ausf/consumer"
	ausf_context "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/ausf/ueauthentication"
	"github.com/omec-project/ausf/util"
	"github.com/omec-project/config5g/proto/client"
	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
	"github.com/omec-project/http2_util"
	"github.com/omec-project/logger_util"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/path_util"
	pathUtilLogger "github.com/omec-project/util/path_util/logger"
)

type AUSF struct{}

type (
	// Config information.
	Config struct {
		ausfcfg string
	}
)

var config Config

var ausfCLi = []cli.Flag{
	cli.StringFlag{
		Name:  "free5gccfg",
		Usage: "common config file",
	},
	cli.StringFlag{
		Name:  "ausfcfg",
		Usage: "config file",
	},
}

var (
	KeepAliveTimer      *time.Timer
	KeepAliveTimerMutex sync.Mutex
)

var initLog *logrus.Entry

func init() {
	initLog = logger.InitLog
}

var ConfigPodTrigger chan bool

func init() {
	ConfigPodTrigger = make(chan bool)
}

func (*AUSF) GetCliCmd() (flags []cli.Flag) {
	return ausfCLi
}

func (ausf *AUSF) Initialize(c *cli.Context) error {
	config = Config{
		ausfcfg: c.String("ausfcfg"),
	}

	if config.ausfcfg != "" {
		if err := factory.InitConfigFactory(config.ausfcfg); err != nil {
			return err
		}
	} else {
		DefaultAusfConfigPath := path_util.Free5gcPath("free5gc/config/ausfcfg.yaml")
		if err := factory.InitConfigFactory(DefaultAusfConfigPath); err != nil {
			return err
		}
	}

	ausf.setLogLevel()

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	roc := os.Getenv("MANAGED_BY_CONFIG_POD")
	if roc == "true" {
		initLog.Infoln("MANAGED_BY_CONFIG_POD is true")
		commChannel := client.ConfigWatcher()
		go ausf.updateConfig(commChannel)
	} else {
		go func() {
			initLog.Infoln("Use helm chart config ")
			ConfigPodTrigger <- true
		}()
	}
	return nil
}

func (ausf *AUSF) setLogLevel() {
	if factory.AusfConfig.Logger == nil {
		initLog.Warnln("AUSF config without log level setting!!!")
		return
	}

	if factory.AusfConfig.Logger.AUSF != nil {
		if factory.AusfConfig.Logger.AUSF.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AusfConfig.Logger.AUSF.DebugLevel); err != nil {
				initLog.Warnf("AUSF Log level [%s] is invalid, set to [info] level",
					factory.AusfConfig.Logger.AUSF.DebugLevel)
				logger.SetLogLevel(logrus.InfoLevel)
			} else {
				initLog.Infof("AUSF Log level is set to [%s] level", level)
				logger.SetLogLevel(level)
			}
		} else {
			initLog.Warnln("AUSF Log level not set. Default set to [info] level")
			logger.SetLogLevel(logrus.InfoLevel)
		}
		logger.SetReportCaller(factory.AusfConfig.Logger.AUSF.ReportCaller)
	}

	if factory.AusfConfig.Logger.PathUtil != nil {
		if factory.AusfConfig.Logger.PathUtil.DebugLevel != "" {
			if level, err := logrus.ParseLevel(factory.AusfConfig.Logger.PathUtil.DebugLevel); err != nil {
				pathUtilLogger.PathLog.Warnf("PathUtil Log level [%s] is invalid, set to [info] level",
					factory.AusfConfig.Logger.PathUtil.DebugLevel)
				pathUtilLogger.SetLogLevel(logrus.InfoLevel)
			} else {
				pathUtilLogger.SetLogLevel(level)
			}
		} else {
			pathUtilLogger.PathLog.Warnln("PathUtil Log level not set. Default set to [info] level")
			pathUtilLogger.SetLogLevel(logrus.InfoLevel)
		}
		pathUtilLogger.SetReportCaller(factory.AusfConfig.Logger.PathUtil.ReportCaller)
	}
}

func (ausf *AUSF) FilterCli(c *cli.Context) (args []string) {
	for _, flag := range ausf.GetCliCmd() {
		name := flag.GetName()
		value := fmt.Sprint(c.Generic(name))
		if value == "" {
			continue
		}

		args = append(args, "--"+name, value)
	}
	return args
}

func (ausf *AUSF) updateConfig(commChannel chan *protos.NetworkSliceResponse) bool {
	var minConfig bool
	context := ausf_context.GetSelf()
	for rsp := range commChannel {
		logger.GrpcLog.Infoln("Received updateConfig in the ausf app : ", rsp)
		for _, ns := range rsp.NetworkSlice {
			logger.GrpcLog.Infoln("Network Slice Name ", ns.Name)
			if ns.Site != nil {
				temp := models.PlmnId{}
				var found bool = false
				logger.GrpcLog.Infoln("Network Slice has site name present ")
				site := ns.Site
				logger.GrpcLog.Infoln("Site name ", site.SiteName)
				if site.Plmn != nil {
					temp.Mcc = site.Plmn.Mcc
					temp.Mnc = site.Plmn.Mnc
					logger.GrpcLog.Infoln("Plmn mcc ", site.Plmn.Mcc)
					for _, item := range context.PlmnList {
						if item.Mcc == temp.Mcc && item.Mnc == temp.Mnc {
							found = true
							break
						}
					}
					if !found {
						context.PlmnList = append(context.PlmnList, temp)
						logger.GrpcLog.Infoln("Plmn added in the context", context.PlmnList)
					}
				} else {
					logger.GrpcLog.Infoln("Plmn not present in the message ")
				}
			}
		}
		if !minConfig {
			// first slice Created
			if len(context.PlmnList) > 0 {
				minConfig = true
				ConfigPodTrigger <- true
				logger.GrpcLog.Infoln("Send config trigger to main routine first time config")
			}
		} else {
			// all slices deleted
			if len(context.PlmnList) == 0 {
				minConfig = false
				ConfigPodTrigger <- false
				logger.GrpcLog.Infoln("Send config trigger to main routine config deleted")
			} else {
				ConfigPodTrigger <- true
				logger.GrpcLog.Infoln("Send config trigger to main routine config updated")
			}
		}
	}
	return true
}

func (ausf *AUSF) Start() {
	initLog.Infoln("Server started")

	router := logger_util.NewGinWithLogrus(logger.GinLog)
	ueauthentication.AddService(router)

	ausf_context.Init()
	self := ausf_context.GetSelf()
	// Register to NRF
	go ausf.registerNF()

	ausfLogPath := util.AusfLogPath

	addr := fmt.Sprintf("%s:%d", self.BindingIPv4, self.SBIPort)

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		ausf.Terminate()
		os.Exit(0)
	}()

	server, err := http2_util.NewServer(addr, ausfLogPath, router)
	if server == nil {
		initLog.Errorf("Initialize HTTP server failed: %+v", err)
		return
	}

	if err != nil {
		initLog.Warnf("Initialize HTTP server: +%v", err)
	}

	serverScheme := factory.AusfConfig.Configuration.Sbi.Scheme
	if serverScheme == "http" {
		err = server.ListenAndServe()
	} else if serverScheme == "https" {
		err = server.ListenAndServeTLS(util.AusfPemPath, util.AusfKeyPath)
	}

	if err != nil {
		initLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (ausf *AUSF) Exec(c *cli.Context) error {
	initLog.Traceln("args:", c.String("ausfcfg"))
	args := ausf.FilterCli(c)
	initLog.Traceln("filter: ", args)
	command := exec.Command("./ausf", args...)

	stdout, err := command.StdoutPipe()
	if err != nil {
		initLog.Fatalln(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			fmt.Println(in.Text())
		}
		wg.Done()
	}()

	stderr, err := command.StderrPipe()
	if err != nil {
		initLog.Fatalln(err)
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			fmt.Println(in.Text())
		}
		wg.Done()
	}()

	go func() {
		startErr := command.Start()
		if startErr != nil {
			initLog.Fatalln(startErr)
		}
		wg.Done()
	}()

	wg.Wait()

	return err
}

func (ausf *AUSF) Terminate() {
	logger.InitLog.Infof("Terminating AUSF...")
	// deregister with NRF
	problemDetails, err := consumer.SendDeregisterNFInstance()
	if problemDetails != nil {
		logger.InitLog.Errorf("Deregister NF instance Failed Problem[%+v]", problemDetails)
	} else if err != nil {
		logger.InitLog.Errorf("Deregister NF instance Error[%+v]", err)
	} else {
		logger.InitLog.Infof("Deregister from NRF successfully")
	}

	logger.InitLog.Infof("AUSF terminated")
}

func (ausf *AUSF) StartKeepAliveTimer(nfProfile models.NfProfile) {
	KeepAliveTimerMutex.Lock()
	defer KeepAliveTimerMutex.Unlock()
	ausf.StopKeepAliveTimer()
	if nfProfile.HeartBeatTimer == 0 {
		nfProfile.HeartBeatTimer = 60
	}
	logger.InitLog.Infof("Started KeepAlive Timer: %v sec", nfProfile.HeartBeatTimer)
	//AfterFunc starts timer and waits for KeepAliveTimer to elapse and then calls ausf.UpdateNF function
	KeepAliveTimer = time.AfterFunc(time.Duration(nfProfile.HeartBeatTimer)*time.Second, ausf.UpdateNF)
}

func (ausf *AUSF) StopKeepAliveTimer() {
	if KeepAliveTimer != nil {
		logger.InitLog.Infof("Stopped KeepAlive Timer.")
		KeepAliveTimer.Stop()
		KeepAliveTimer = nil
	}
}

func (ausf *AUSF) BuildAndSendRegisterNFInstance() (models.NfProfile, error) {
	self := context.GetSelf()
	profile, err := consumer.BuildNFInstance(self)
	if err != nil {
		initLog.Errorf("Build AUSF Profile Error: %v", err)
		return profile, err
	}
	initLog.Infof("Pcf Profile Registering to NRF: %v", profile)
	//Indefinite attempt to register until success
	profile, _, self.NfId, err = consumer.SendRegisterNFInstance(self.NrfUri, self.NfId, profile)
	return profile, err
}

// UpdateNF is the callback function, this is called when keepalivetimer elapsed
func (ausf *AUSF) UpdateNF() {
	KeepAliveTimerMutex.Lock()
	defer KeepAliveTimerMutex.Unlock()
	if KeepAliveTimer == nil {
		initLog.Warnf("KeepAlive timer has been stopped.")
		return
	}
	//setting default value 30 sec
	var heartBeatTimer int32 = 60
	pitem := models.PatchItem{
		Op:    "replace",
		Path:  "/nfStatus",
		Value: "REGISTERED",
	}
	var patchItem []models.PatchItem
	patchItem = append(patchItem, pitem)
	nfProfile, problemDetails, err := consumer.SendUpdateNFInstance(patchItem)
	if problemDetails != nil {
		initLog.Errorf("AUSF update to NRF ProblemDetails[%v]", problemDetails)
		//5xx response from NRF, 404 Not Found, 400 Bad Request
		if (problemDetails.Status/100) == 5 ||
			problemDetails.Status == 404 || problemDetails.Status == 400 {
			//register with NRF full profile
			nfProfile, err = ausf.BuildAndSendRegisterNFInstance()
			if err != nil {
				initLog.Errorf("AUSF register to NRF Error[%s]", err.Error())
			}
		}
	} else if err != nil {
		initLog.Errorf("AUSF update to NRF Error[%s]", err.Error())
		nfProfile, err = ausf.BuildAndSendRegisterNFInstance()
		if err != nil {
			initLog.Errorf("AUSF register to NRF Error[%s]", err.Error())
		}
	}

	if nfProfile.HeartBeatTimer != 0 {
		// use hearbeattimer value with received timer value from NRF
		heartBeatTimer = nfProfile.HeartBeatTimer
	}
	logger.InitLog.Debugf("Restarted KeepAlive Timer: %v sec", heartBeatTimer)
	//restart timer with received HeartBeatTimer value
	KeepAliveTimer = time.AfterFunc(time.Duration(heartBeatTimer)*time.Second, ausf.UpdateNF)
}

func (ausf *AUSF) registerNF() {
	for msg := range ConfigPodTrigger {
		initLog.Infof("Minimum configuration from config pod available %v", msg)
		self := ausf_context.GetSelf()
		profile, err := consumer.BuildNFInstance(self)
		if err != nil {
			initLog.Error("Build AUSF Profile Error")
		}
		var prof models.NfProfile
		prof, _, self.NfId, err = consumer.SendRegisterNFInstance(self.NrfUri, self.NfId, profile)
		if err != nil {
			initLog.Errorf("AUSF register to NRF Error[%s]", err.Error())
		} else {
			//stop keepAliveTimer if its running
			ausf.StartKeepAliveTimer(prof)
			logger.CfgLog.Infof("Sent Register NF Instance with updated profile")
		}
	}
}
