// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2024 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nfregistration

import (
	"context"
	"sync"
	"time"

	"github.com/omec-project/ausf/consumer"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi/models"
)

const RETRY_TIME = 10

var (
	keepAliveTimer      *time.Timer
	keepAliveTimerMutex sync.Mutex
	registerCtxMutex    sync.Mutex
)

const (
	DEFAULT_HEARTBEAT_TIMER int32 = 60
)

// StartNfRegistrationService starts the registration service. If the new config is empty, the NF
// deregisters from the NRF. Else, it registers to the NRF. It cancels registerCancel to ensure
// that only one registration process runs at the time.
func StartNfRegistrationService(ctx context.Context, plmnConfigChan <-chan []models.PlmnId) {
	var registerCancel context.CancelFunc
	var registerCtx context.Context

	for {
		select {
		case <-ctx.Done():
			if registerCancel != nil {
				registerCancel()
			}
			logger.NrfRegistrationLog.Infoln("NF registration service shutting down")
			return
		case newPlmnConfig := <-plmnConfigChan:
			// Cancel current sync if running
			if registerCancel != nil {
				logger.NrfRegistrationLog.Infoln("NF registration context cancelled")
				registerCancel()
			}

			if len(newPlmnConfig) == 0 {
				logger.NrfRegistrationLog.Infoln("PLMN config is empty. AUSF will degister")
				DeregisterNF()
			} else {
				logger.NrfRegistrationLog.Infoln("PLMN config is not empty. AUSF will update registration")
				registerCtx, registerCancel = context.WithCancel(context.Background())
				// Create new cancellable context for this registration
				go registerNF(registerCtx, newPlmnConfig)
			}
		}
	}
}

// registerNF sends a RegisterNFInstance. If it fails, it keeps retrying, until the context is cancelled by StartNfRegistrationService
var registerNF = func(registerCtx context.Context, newPlmnConfig []models.PlmnId) {
	registerCtxMutex.Lock()
	defer registerCtxMutex.Unlock()
	for {
		select {
		case <-registerCtx.Done():
			logger.NrfRegistrationLog.Infoln("no-op. Registration context was cancelled")
			return
		default:
			nfProfile, _, err := consumer.SendRegisterNFInstance(newPlmnConfig)
			if err != nil {
				logger.NrfRegistrationLog.Errorln("register AUSF instance to NRF failed. Will retry.", err.Error())
				time.Sleep(RETRY_TIME * time.Second)
				continue
			}
			logger.NrfRegistrationLog.Infoln("register AUSF instance to NRF with updated profile succeeded")
			startKeepAliveTimer(nfProfile.HeartBeatTimer, newPlmnConfig)
			return
		}
	}
}

// heartbeatNF is the callback function, this is called when keepalivetimer elapsed.
// It sends a Update NF instance to the NRF. If it fails, it tries to register again.
// keepAliveTimer is restarted at the end.
func heartbeatNF(plmnConfig []models.PlmnId) {
	keepAliveTimerMutex.Lock()
	if keepAliveTimer == nil {
		keepAliveTimerMutex.Unlock()
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
		logger.NrfRegistrationLog.Debugln("NF heartbeat failed. Trying to register again")
		nfProfile, _, err = consumer.SendRegisterNFInstance(plmnConfig)
		if err != nil {
			logger.NrfRegistrationLog.Errorln("register AUSF instance error:", err.Error())
		}
		logger.NrfRegistrationLog.Infoln("register AUSF instance to NRF with updated profile succeeded")
	} else {
		logger.NrfRegistrationLog.Infoln("AUSF update NF instance (heartbeat) succeeded")
	}
	startKeepAliveTimer(nfProfile.HeartBeatTimer, plmnConfig)
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
	err := consumer.SendDeregisterNFInstance()
	if err != nil {
		logger.NrfRegistrationLog.Warnln("deregister instance from NRF error:", err.Error())
		return
	}
	logger.NrfRegistrationLog.Infoln("deregister instance from NRF successful")
}

func startKeepAliveTimer(profileHeartbeatTimer int32, plmnConfig []models.PlmnId) {
	keepAliveTimerMutex.Lock()
	defer keepAliveTimerMutex.Unlock()
	stopKeepAliveTimer()
	heartbeatTimer := DEFAULT_HEARTBEAT_TIMER
	if profileHeartbeatTimer != 0 {
		heartbeatTimer = profileHeartbeatTimer
	}
	heartbeatFunction := func() { heartbeatNF(plmnConfig) }
	// AfterFunc starts timer and waits for keepAliveTimer to elapse and then calls heartbeatNF function
	keepAliveTimer = time.AfterFunc(time.Duration(heartbeatTimer)*time.Second, heartbeatFunction)
	logger.NrfRegistrationLog.Infof("started heartbeat timer: %v sec", heartbeatTimer)
}

func stopKeepAliveTimer() {
	if keepAliveTimer != nil {
		keepAliveTimer.Stop()
		keepAliveTimer = nil
		logger.NrfRegistrationLog.Infoln("stopped heartbeat timer")
	}
}
