// SPDX-FileCopyrightText: 2025 Canonical Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//
/*
 * NRF Registration Unit Testcases
 *
 */
package nfregistration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omec-project/ausf/consumer"
	"github.com/omec-project/openapi/models"
)

func TestNfRegistrationService_EmptyConfig_DeregisterNF_StopTimer(t *testing.T) {
	var isDeregisterNFCalled bool
	testCases := []struct {
		name                         string
		sendDeregisterNFInstanceMock func() (*models.ProblemDetails, error)
	}{
		{
			name: "Success",
			sendDeregisterNFInstanceMock: func() (*models.ProblemDetails, error) {
				isDeregisterNFCalled = true
				return nil, nil
			},
		},
		{
			name: "ErrorInDeregisterNFInstance",
			sendDeregisterNFInstanceMock: func() (*models.ProblemDetails, error) {
				isDeregisterNFCalled = true
				return nil, errors.New("mock error")
			},
		},
		{
			name: "ProblemDetailsInDeregisterNFInstance",
			sendDeregisterNFInstanceMock: func() (*models.ProblemDetails, error) {
				isDeregisterNFCalled = true
				return &models.ProblemDetails{}, nil
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keepAliveTimer = time.NewTimer(60 * time.Second)
			isRegisterNFCalled := false
			isDeregisterNFCalled = false
			originalDeregisterNF := consumer.SendDeregisterNFInstance
			originalRegisterNF := registerNF
			defer func() {
				consumer.SendDeregisterNFInstance = originalDeregisterNF
				registerNF = originalRegisterNF
				if keepAliveTimer != nil {
					keepAliveTimer.Stop()
				}
			}()

			consumer.SendDeregisterNFInstance = tc.sendDeregisterNFInstanceMock

			registerNF = func(ctx context.Context, newPlmnConfig []models.PlmnId) {
				isRegisterNFCalled = true
			}

			ch := make(chan []models.PlmnId, 1)
			ctx, registerCancel := context.WithCancel(context.Background())
			defer registerCancel()
			go StartNfRegistrationService(ctx, ch)
			ch <- []models.PlmnId{}

			time.Sleep(1 * time.Second)

			if keepAliveTimer != nil {
				t.Errorf("expected keepAliveTimer to be nil after stopKeepAliveTimer")
			}

			if !isDeregisterNFCalled {
				t.Errorf("expected SendDeregisterNFInstance to be called")
			}

			if isRegisterNFCalled {
				t.Errorf("expected registerNF not to be called")
			}
		})
	}
}

func TestNfRegistrationService_ConfigChanged_RegisterNFSuccess_StartTimer(t *testing.T) {
	keepAliveTimer = nil
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	consumer.SendRegisterNFInstance = func(plmnConfig []models.PlmnId) (models.NfProfile, string, error) {
		profile := models.NfProfile{HeartBeatTimer: 60}
		return profile, "", nil
	}

	ch := make(chan []models.PlmnId, 1)
	ctx, registerCancel := context.WithCancel(context.Background())
	defer registerCancel()
	go StartNfRegistrationService(ctx, ch)
	ch <- []models.PlmnId{{Mcc: "001", Mnc: "01"}}

	time.Sleep(1 * time.Second)
	if keepAliveTimer == nil {
		t.Error("expected keepAliveTimer to be initialized by startKeepAliveTimer")
	}
}

func TestNfRegistrationService_ConfigChanged_RegisterNFFails(t *testing.T) {
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	called := 0
	consumer.SendRegisterNFInstance = func(plmnConfig []models.PlmnId) (models.NfProfile, string, error) {
		profile := models.NfProfile{HeartBeatTimer: 60}
		called++
		return profile, "", errors.New("mock error")
	}

	ch := make(chan []models.PlmnId, 1)
	ctx, registerCancel := context.WithCancel(context.Background())
	defer registerCancel()
	go StartNfRegistrationService(ctx, ch)
	ch <- []models.PlmnId{{Mcc: "001", Mnc: "01"}}

	time.Sleep((RETRY_TIME*2 + 1) * time.Second)

	if called < 2 {
		t.Error("Expected to retry register to NRF")
	}
}

func TestHeartbeatNF_Success(t *testing.T) {
	keepAliveTimer = time.NewTimer(60 * time.Second)
	calledRegister := false
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	originalSendUpdateNFInstance := consumer.SendUpdateNFInstance
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		consumer.SendUpdateNFInstance = originalSendUpdateNFInstance
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	consumer.SendUpdateNFInstance = func(patchItem []models.PatchItem) (models.NfProfile, *models.ProblemDetails, error) {
		return models.NfProfile{}, nil, nil
	}
	consumer.SendRegisterNFInstance = func(plmnConfig []models.PlmnId) (models.NfProfile, string, error) {
		profile := models.NfProfile{HeartBeatTimer: 60}
		return profile, "", nil
	}
	plmnConfig := []models.PlmnId{}
	heartbeatNF(plmnConfig)

	if calledRegister {
		t.Errorf("expected registerNF to be called on error")
	}

	if keepAliveTimer == nil {
		t.Error("expected keepAliveTimer to be initialized by startKeepAliveTimer")
	}
}

func TestHeartbeatNF_RegistersOnError(t *testing.T) {
	keepAliveTimer = time.NewTimer(60 * time.Second)
	calledRegister := false
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	originalSendUpdateNFInstance := consumer.SendUpdateNFInstance
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		consumer.SendUpdateNFInstance = originalSendUpdateNFInstance
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	consumer.SendUpdateNFInstance = func(patchItem []models.PatchItem) (models.NfProfile, *models.ProblemDetails, error) {
		return models.NfProfile{}, nil, errors.New("mock error")
	}

	consumer.SendRegisterNFInstance = func(plmnConfig []models.PlmnId) (models.NfProfile, string, error) {
		profile := models.NfProfile{HeartBeatTimer: 60}
		calledRegister = true
		return profile, "", nil
	}

	plmnConfig := []models.PlmnId{}
	heartbeatNF(plmnConfig)

	if !calledRegister {
		t.Errorf("expected registerNF to be called on error")
	}

	if keepAliveTimer == nil {
		t.Error("expected keepAliveTimer to be initialized by startKeepAliveTimer")
	}
}
