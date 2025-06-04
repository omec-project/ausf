// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2024 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nrfregistration

import (
	"context"
	"sync"
	"time"

	"github.com/omec-project/ausf/consumer"
	ausfContext "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi/models"
)

var (
	keepAliveTimer      *time.Timer
	keepAliveTimerMutex sync.Mutex
	registerCtx         context.Context
	registerCancel      context.CancelFunc
	registerCtxMutex    sync.Mutex
)

const DEFAULT_HEARTBEAT_TIMER int32 = 60

// HandleNewConfig receives the new config. If the new config is empty, the NF deregisters from the NRF.
// Else, it registers to the NRF. It cancels registerCancel to ensure that only one registration
// process runs at the time.
var HandleNewConfig = func(newPlmnConfig []models.PlmnId) {
	registerCtxMutex.Lock()
	defer registerCtxMutex.Unlock()

	if registerCancel != nil {
		registerCancel()
		logger.NrfRegistrationLog.Infoln("NF registration context cancelled")
	}

	if len(newPlmnConfig) == 0 {
		logger.NrfRegistrationLog.Debugln("PLMN config is empty. AUSF will degister")
		DeregisterNF()
	} else {
		logger.NrfRegistrationLog.Debugln("PLMN config is not empty. AUSF will update registration")
		// Create new cancellable context for this registration
		registerCtx, registerCancel = context.WithCancel(context.Background())
		go registerNF(registerCtx)
	}
}

// registerNF sends a RegisterNFInstance. If it fails, it keeps retrying, until the context is cancelled by HandleNewConfig.
var registerNF = func(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.NrfRegistrationLog.Infoln("register AUSF instance to NRF cancelled due to new configuration")
			return
		default:
			ausfContext := ausfContext.GetSelf()
			nfProfile, err := consumer.BuildNFInstance(ausfContext)
			if err != nil {
				logger.NrfRegistrationLog.Warnln("build AUSF nfProfile failed:", err)
			}
			nfProfile, _, ausfContext.NfId, err = consumer.SendRegisterNFInstance(ausfContext.NrfUri, ausfContext.NfId, nfProfile)
			if err != nil {
				logger.NrfRegistrationLog.Errorln("register AUSF instance to NRF error:", err.Error())
				time.Sleep(10 * time.Second)
				continue
			}
			logger.NrfRegistrationLog.Infoln("register AUSF instance to NRF with updated profile succeeded")
			startKeepAliveTimer(nfProfile)
			return
		}
	}
}

func buildAndSendRegisterNFInstance() (models.NfProfile, error) {
	self := ausfContext.GetSelf()
	nfProfile, err := consumer.BuildNFInstance(self)
	if err != nil {
		return nfProfile, err
	}

	nfProfile, _, self.NfId, err = consumer.SendRegisterNFInstance(self.NrfUri, self.NfId, nfProfile)
	if err != nil {
		return nfProfile, err
	}
	logger.NrfRegistrationLog.Infoln("register AUSF instance to NRF with updated profile succeeded")
	return nfProfile, nil
}

// heartbeatNF is the callback function, this is called when keepalivetimer elapsed.
// It sends a Update NF instance to the NRF. If it fails, it tries to register again.
// keepAliveTimer is restarted at the end.
func heartbeatNF() {
	keepAliveTimerMutex.Lock()
	if keepAliveTimer == nil {
		logger.NrfRegistrationLog.Infoln("heartbeat timer has been stopped, heartbeat will not be sent to NRF")
		return
	}
	keepAliveTimerMutex.Unlock()

	patchItem := []models.PatchItem{
		{
			Op:    "replace",
			Path:  "/nfStatus",
			Value: "REGISTERED",
		},
	}
	nfProfile, problemDetails, err := consumer.SendUpdateNFInstance(patchItem)

	if shouldRegister(problemDetails, err) {
		nfProfile, err = buildAndSendRegisterNFInstance()
		if err != nil {
			logger.NrfRegistrationLog.Errorln("register AUSF instance error:", err.Error())
		}
	} else {
		logger.NrfRegistrationLog.Infoln("AUSF update NF instance (heartbeat) succeeded")
	}
	startKeepAliveTimer(nfProfile)
}

func shouldRegister(problemDetails *models.ProblemDetails, err error) bool {
	if problemDetails != nil {
		logger.NrfRegistrationLog.Warnln("AUSF update NF instance (heartbeat) problem details:", problemDetails)
		status := problemDetails.Status
		return (status/100) == 5 || status == 404 || status == 400
	}
	if err != nil {
		logger.NrfRegistrationLog.Warnln("AUSF update NF instance (heartbeat) error:", err.Error())
		return true
	}
	return false
}

var DeregisterNF = func() {
	keepAliveTimerMutex.Lock()
	stopKeepAliveTimer()
	keepAliveTimerMutex.Unlock()
	problemDetails, err := consumer.SendDeregisterNFInstance()
	if err != nil {
		logger.NrfRegistrationLog.Warnln("deregister instance from NRF error:", err.Error())
		return
	}
	if problemDetails != nil {
		logger.NrfRegistrationLog.Warnln("deregister instance from NRF problem details:", problemDetails)
		return
	}
	logger.NrfRegistrationLog.Infoln("deregister instance from NRF successful")
}

func startKeepAliveTimer(nfProfile models.NfProfile) {
	keepAliveTimerMutex.Lock()
	defer keepAliveTimerMutex.Unlock()
	stopKeepAliveTimer()
	heartbeatTimer := DEFAULT_HEARTBEAT_TIMER
	if nfProfile.HeartBeatTimer != 0 {
		heartbeatTimer = nfProfile.HeartBeatTimer
	}
	// AfterFunc starts timer and waits for keepAliveTimer to elapse and then calls heartbeatNF function
	keepAliveTimer = time.AfterFunc(time.Duration(heartbeatTimer)*time.Second, heartbeatNF)
	logger.NrfRegistrationLog.Infof("started heartbeat timer: %v sec", heartbeatTimer)
}

func stopKeepAliveTimer() {
	if keepAliveTimer != nil {
		keepAliveTimer.Stop()
		keepAliveTimer = nil
		logger.NrfRegistrationLog.Infoln("stopped heartbeat timer")
	}
}
