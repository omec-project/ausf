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
	"sync"
	"syscall"
	"time"

	"github.com/omec-project/ausf/callback"
	"github.com/omec-project/ausf/consumer"
	"github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/ausf/metrics"
	"github.com/omec-project/ausf/ueauthentication"
	"github.com/omec-project/ausf/util"
	grpcClient "github.com/omec-project/config5g/proto/client"
	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
	"github.com/omec-project/openapi/models"
	nrfCache "github.com/omec-project/openapi/nrfcache"
	"github.com/omec-project/util/http2_util"
	utilLogger "github.com/omec-project/util/logger"
	"github.com/omec-project/util/path_util"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

	if os.Getenv("MANAGED_BY_CONFIG_POD") == "true" {
		logger.InitLog.Infoln("MANAGED_BY_CONFIG_POD is true")
		client, err := grpcClient.ConnectToConfigServer(factory.AusfConfig.Configuration.WebuiUri)
		if err != nil {
			go updateConfig(client, ausf)
		}
		return err
	} else {
		go func() {
			logger.InitLog.Infoln("use helm chart config")
			ConfigPodTrigger <- true
		}()
	}
	return nil
}

// updateConfig connects the config pod GRPC server and subscribes the config changes
// then updates AUSF configuration
func updateConfig(client grpcClient.ConfClient, ausf *AUSF) {
	var stream protos.ConfigService_NetworkSliceSubscribeClient
	var err error
	var configChannel chan *protos.NetworkSliceResponse
	for {
		if client != nil {
			stream, err = client.CheckGrpcConnectivity()
			if err != nil {
				logger.InitLog.Errorf("%v", err)
				if stream != nil {
					time.Sleep(time.Second * 30)
					continue
				} else {
					err = client.GetConfigClientConn().Close()
					if err != nil {
						logger.InitLog.Debugf("failing ConfigClient is not closed properly: %+v", err)
					}
					client = nil
					continue
				}
			}
			if configChannel == nil {
				configChannel = client.PublishOnConfigChange(true, stream)
				go ausf.updateConfig(configChannel)
			}

		} else {
			client, err = grpcClient.ConnectToConfigServer(factory.AusfConfig.Configuration.WebuiUri)
			if err != nil {
				logger.InitLog.Errorf("%+v", err)
			}
			continue
		}
	}
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
	context := context.GetSelf()
	for rsp := range commChannel {
		logger.GrpcLog.Infoln("received updateConfig in the ausf app:", rsp)
		for _, ns := range rsp.NetworkSlice {
			logger.GrpcLog.Infoln("network Slice Name", ns.Name)
			if ns.Site != nil {
				temp := models.PlmnId{}
				var found bool = false
				logger.GrpcLog.Infoln("network slice has site name present")
				site := ns.Site
				logger.GrpcLog.Infoln("site name", site.SiteName)
				if site.Plmn != nil {
					temp.Mcc = site.Plmn.Mcc
					temp.Mnc = site.Plmn.Mnc
					logger.GrpcLog.Infoln("plmn mcc", site.Plmn.Mcc)
					for _, item := range context.PlmnList {
						if item.Mcc == temp.Mcc && item.Mnc == temp.Mnc {
							found = true
							break
						}
					}
					if !found {
						context.PlmnList = append(context.PlmnList, temp)
						logger.GrpcLog.Infoln("plmn added in the context", context.PlmnList)
					}
				} else {
					logger.GrpcLog.Infoln("plmn not present in the message ")
				}
			}
		}
		if !minConfig {
			// first slice Created
			if len(context.PlmnList) > 0 {
				minConfig = true
				ConfigPodTrigger <- true
				logger.GrpcLog.Infoln("send config trigger to main routine first time config")
			}
		} else {
			// all slices deleted
			if len(context.PlmnList) == 0 {
				minConfig = false
				ConfigPodTrigger <- false
				logger.GrpcLog.Infoln("send config trigger to main routine config deleted")
			} else {
				ConfigPodTrigger <- true
				logger.GrpcLog.Infoln("send config trigger to main routine config updated")
			}
		}
	}
	return true
}

func (ausf *AUSF) Start() {
	logger.InitLog.Infoln("server started")

	router := utilLogger.NewGinWithZap(logger.GinLog)
	ueauthentication.AddService(router)
	callback.AddService(router)

	go metrics.InitMetrics()

	context.Init()
	self := context.GetSelf()

	ausfLogPath := util.AusfLogPath

	addr := fmt.Sprintf("%s:%d", self.BindingIPv4, self.SBIPort)

	if self.EnableNrfCaching {
		logger.InitLog.Infoln("enable NRF caching feature")
		nrfCache.InitNrfCaching(self.NrfCacheEvictionInterval*time.Second, consumer.SendNfDiscoveryToNrf)
	}
	// Register to NRF
	go ausf.RegisterNF()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		ausf.Terminate()
		os.Exit(0)
	}()

	server, err := http2_util.NewServer(addr, ausfLogPath, router)
	if server == nil {
		logger.InitLog.Errorf("initialize HTTP server failed: %v", err)
		return
	}

	if err != nil {
		logger.InitLog.Warnf("initialize HTTP server: %v", err)
	}

	serverScheme := factory.AusfConfig.Configuration.Sbi.Scheme
	if serverScheme == "http" {
		err = server.ListenAndServe()
	} else if serverScheme == "https" {
		err = server.ListenAndServeTLS(self.PEM, self.Key)
	}

	if err != nil {
		logger.InitLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (ausf *AUSF) Exec(c *cli.Context) error {
	logger.InitLog.Debugln("args:", c.String("ausfcfg"))
	args := ausf.FilterCli(c)
	logger.InitLog.Debugln("filter:", args)
	command := exec.Command("./ausf", args...)

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
	// deregister with NRF
	problemDetails, err := consumer.SendDeregisterNFInstance()
	if problemDetails != nil {
		logger.InitLog.Errorf("deregister NF instance Failed Problem[%+v]", problemDetails)
	} else if err != nil {
		logger.InitLog.Errorf("deregister NF instance Error[%+v]", err)
	} else {
		logger.InitLog.Infoln("deregister from NRF successfully")
	}

	logger.InitLog.Infoln("AUSF terminated")
}

func (ausf *AUSF) StartKeepAliveTimer(nfProfile models.NfProfile) {
	KeepAliveTimerMutex.Lock()
	defer KeepAliveTimerMutex.Unlock()
	ausf.StopKeepAliveTimer()
	if nfProfile.HeartBeatTimer == 0 {
		nfProfile.HeartBeatTimer = 60
	}
	logger.InitLog.Infof("started KeepAlive Timer: %v sec", nfProfile.HeartBeatTimer)
	//AfterFunc starts timer and waits for KeepAliveTimer to elapse and then calls ausf.UpdateNF function
	KeepAliveTimer = time.AfterFunc(time.Duration(nfProfile.HeartBeatTimer)*time.Second, ausf.UpdateNF)
}

func (ausf *AUSF) StopKeepAliveTimer() {
	if KeepAliveTimer != nil {
		logger.InitLog.Infoln("stopped KeepAlive Timer")
		KeepAliveTimer.Stop()
		KeepAliveTimer = nil
	}
}

func (ausf *AUSF) BuildAndSendRegisterNFInstance() (models.NfProfile, error) {
	self := context.GetSelf()
	profile, err := consumer.BuildNFInstance(self)
	if err != nil {
		logger.InitLog.Errorf("build AUSF Profile Error: %v", err)
		return profile, err
	}
	logger.InitLog.Infof("AUSF Profile Registering to NRF: %v", profile)
	//Indefinite attempt to register until success
	profile, _, self.NfId, err = consumer.SendRegisterNFInstance(self.NrfUri, self.NfId, profile)
	return profile, err
}

// UpdateNF is the callback function, this is called when keepalivetimer elapsed
func (ausf *AUSF) UpdateNF() {
	KeepAliveTimerMutex.Lock()
	defer KeepAliveTimerMutex.Unlock()
	if KeepAliveTimer == nil {
		logger.InitLog.Warnln("KeepAlive timer has been stopped")
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
		logger.InitLog.Errorf("AUSF update to NRF ProblemDetails[%v]", problemDetails)
		//5xx response from NRF, 404 Not Found, 400 Bad Request
		if (problemDetails.Status/100) == 5 ||
			problemDetails.Status == 404 || problemDetails.Status == 400 {
			//register with NRF full profile
			nfProfile, err = ausf.BuildAndSendRegisterNFInstance()
			if err != nil {
				logger.InitLog.Errorf("AUSF register to NRF Error[%s]", err.Error())
			}
		}
	} else if err != nil {
		logger.InitLog.Errorf("AUSF update to NRF Error[%s]", err.Error())
		nfProfile, err = ausf.BuildAndSendRegisterNFInstance()
		if err != nil {
			logger.InitLog.Errorf("AUSF register to NRF Error[%s]", err.Error())
		}
	}

	if nfProfile.HeartBeatTimer != 0 {
		// use hearbeattimer value with received timer value from NRF
		heartBeatTimer = nfProfile.HeartBeatTimer
	}
	logger.InitLog.Debugf("restarted KeepAlive Timer: %v sec", heartBeatTimer)
	//restart timer with received HeartBeatTimer value
	KeepAliveTimer = time.AfterFunc(time.Duration(heartBeatTimer)*time.Second, ausf.UpdateNF)
}

func (ausf *AUSF) RegisterNF() {
	for msg := range ConfigPodTrigger {
		if msg {
			logger.InitLog.Infof("minimum configuration from config pod available %v", msg)
			self := context.GetSelf()
			profile, err := consumer.BuildNFInstance(self)
			if err != nil {
				logger.InitLog.Errorln("build AUSF Profile Error")
			}
			var prof models.NfProfile
			prof, _, self.NfId, err = consumer.SendRegisterNFInstance(self.NrfUri, self.NfId, profile)
			if err != nil {
				logger.InitLog.Errorf("AUSF register to NRF Error[%s]", err.Error())
			} else {
				//stop keepAliveTimer if its running
				ausf.StartKeepAliveTimer(prof)
				logger.CfgLog.Infoln("sent Register NF Instance with updated profile")
			}
		} else {
			// stopping keepAlive timer
			KeepAliveTimerMutex.Lock()
			ausf.StopKeepAliveTimer()
			KeepAliveTimerMutex.Unlock()
			logger.InitLog.Infoln("AUSF is not having Minimum Config to Register/Update to NRF")
			problemDetails, err := consumer.SendDeregisterNFInstance()
			if problemDetails != nil {
				logger.InitLog.Errorf("deregister Instance to NRF failed, Problem: [+%v]", problemDetails)
			}
			if err != nil {
				logger.InitLog.Errorf("deregister Instance to NRF Error[%s]", err.Error())
			} else {
				logger.InitLog.Infoln("deregister from NRF successfully")
			}
		}
	}
}
