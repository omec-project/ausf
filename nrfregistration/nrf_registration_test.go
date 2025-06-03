// SPDX-FileCopyrightText: 2025 Canonical Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//
/*
 * NRF Registration Unit Testcases
 *
 */
package nrfregistration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omec-project/ausf/consumer"
	"github.com/omec-project/openapi/models"
)

func TestHandleNewConfig_EmptyConfig_DeregisterNF_StopTimer(t *testing.T) {
	var isSendDeregisterNFInstanceCalled bool
	testCases := []struct {
		name                         string
		sendDeregisterNFInstanceMock func() (*models.ProblemDetails, error)
	}{
		{
			name: "Success",
			sendDeregisterNFInstanceMock: func() (*models.ProblemDetails, error) {
				isSendDeregisterNFInstanceCalled = true
				return nil, nil
			},
		},
		{
			name: "ErrorInDeregisterNFInstance",
			sendDeregisterNFInstanceMock: func() (*models.ProblemDetails, error) {
				isSendDeregisterNFInstanceCalled = true
				return nil, errors.New("mock error")
			},
		},
		{
			name: "ProblemDetailsInDeregisterNFInstance",
			sendDeregisterNFInstanceMock: func() (*models.ProblemDetails, error) {
				isSendDeregisterNFInstanceCalled = true
				return &models.ProblemDetails{}, nil
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keepAliveTimer = time.NewTimer(60 * time.Second)
			isRegisterNFCalled := false
			isSendDeregisterNFInstanceCalled = false
			originalSendDeregisterNFInstance := consumer.SendDeregisterNFInstance
			originalRegisterNF := registerNF
			defer func() {
				consumer.SendDeregisterNFInstance = originalSendDeregisterNFInstance
				registerNF = originalRegisterNF
				if keepAliveTimer != nil {
					keepAliveTimer.Stop()
				}
			}()

			consumer.SendDeregisterNFInstance = tc.sendDeregisterNFInstanceMock

			registerNF = func(ctx context.Context) {
				isRegisterNFCalled = true
			}

			HandleNewConfig([]models.PlmnId{})

			if keepAliveTimer != nil {
				t.Errorf("expected keepAliveTimer to be nil after stopKeepAliveTimer")
			}

			if !isSendDeregisterNFInstanceCalled {
				t.Errorf("expected SendDeregisterNFInstance to be called")
			}

			if isRegisterNFCalled {
				t.Errorf("expected registerNF not to be called")
			}

		})
	}
}

func TestHandleNewConfig_ConfigChanged_RegisterNFSuccess_StartTimer(t *testing.T) {
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	consumer.SendRegisterNFInstance = func(nrfUri string, nfInstanceId string, profile models.NfProfile) (models.NfProfile, string, string, error) {
		profile.HeartBeatTimer = 60
		return profile, "", "", nil
	}

	HandleNewConfig([]models.PlmnId{{Mcc: "001", Mnc: "01"}})
	time.Sleep(1 * time.Second)

	if keepAliveTimer == nil {
		t.Error("expected keepAliveTimer to be initialized by startKeepAliveTimer")
	}

}

func TestHandleNewConfig_ConfigChanged_RegisterNFFails(t *testing.T) {
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	consumer.SendRegisterNFInstance = func(nrfUri string, nfInstanceId string, profile models.NfProfile) (models.NfProfile, string, string, error) {
		profile.HeartBeatTimer = 60
		return profile, "", "", errors.New("mock error")
	}

	// Initial call: should start first registerNF
	HandleNewConfig([]models.PlmnId{{Mcc: "001", Mnc: "01"}})
	time.Sleep(2 * time.Second)

	// Save old context cancel
	registerCtxMutex.Lock()
	oldCancel := registerCancel
	oldContext := registerCtx
	registerCtxMutex.Unlock()

	if oldCancel == nil {
		t.Fatal("expected registerCancel to be set")
	}

	// Second config update: should cancel previous context and start a new one
	HandleNewConfig([]models.PlmnId{{Mcc: "001", Mnc: "02"}})
	time.Sleep(2 * time.Second)

	select {
	case <-oldContext.Done():
		// expected
	default:
		t.Error("expected old context to be cancelled")
	}

	select {
	case <-registerCtx.Done():
		t.Error("expected context to be running")
	default:
		// expected
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

	consumer.SendRegisterNFInstance = func(nrfUri string, nfInstanceId string, profile models.NfProfile) (models.NfProfile, string, string, error) {
		profile.HeartBeatTimer = 60
		calledRegister = true
		return profile, "", "", nil
	}

	heartbeatNF()

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

	consumer.SendRegisterNFInstance = func(nrfUri string, nfInstanceId string, profile models.NfProfile) (models.NfProfile, string, string, error) {
		profile.HeartBeatTimer = 60
		calledRegister = true
		return profile, "", "", nil
	}

	heartbeatNF()

	if !calledRegister {
		t.Errorf("expected registerNF to be called on error")
	}

	if keepAliveTimer == nil {
		t.Error("expected keepAliveTimer to be initialized by startKeepAliveTimer")
	}
}
