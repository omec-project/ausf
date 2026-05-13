// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"net/http"
	"os"
	"sync"
	"testing"

	ausf_context "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/openapi/v2/Nudm_UEAU"
	"github.com/omec-project/openapi/v2/models"
)

var initProducerTestOnce sync.Once

func initProducerTestContext(t *testing.T) {
	t.Helper()
	initProducerTestOnce.Do(func() {
		if err := os.Setenv("POD_IP", "127.0.0.1"); err != nil {
			t.Fatalf("set POD_IP: %v", err)
		}
		if err := factory.InitConfigFactory("../factory/ausfcfg.yaml"); err != nil {
			t.Fatalf("InitConfigFactory: %v", err)
		}
		ausf_context.Init()
	})
}

func TestUeAuthPostRequestProcedure_UnsupportedAuthTypeDoesNotPersistContext(t *testing.T) {
	initProducerTestContext(t)
	originalResolveUdmURL := resolveUdmURL
	originalExecuteGenerateAuthData := executeGenerateAuthData
	defer func() {
		resolveUdmURL = originalResolveUdmURL
		executeGenerateAuthData = originalExecuteGenerateAuthData
	}()

	supiOrSuci := "imsi-001010000000001"
	resolvedSupi := "imsi-001010000000002"
	resolveUdmURL = func(string) string { return "https://udm.example" }
	executeGenerateAuthData = func(_ *Nudm_UEAU.APIClient, _ string, _ models.AuthenticationInfoRequest) (*models.AuthenticationInfoResult, *http.Response, error) {
		result := models.NewAuthenticationInfoResult(models.AuthType("UNSUPPORTED"))
		result.SetSupi(resolvedSupi)
		return result, nil, nil
	}

	response, _, problemDetails := UeAuthPostRequestProcedure(models.AuthenticationInfo{
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		SupiOrSuci:         supiOrSuci,
	})

	if response != nil {
		t.Fatalf("expected nil response, got %+v", response)
	}
	if problemDetails == nil || problemDetails.GetCause() != UPSTREAM_SERVER_ERROR {
		t.Fatalf("expected upstream server error, got %+v", problemDetails)
	}
	if ausf_context.CheckIfSuciSupiPairExists(supiOrSuci) {
		t.Fatalf("unexpected SUCI/SUPI mapping persisted for %s", supiOrSuci)
	}
	if ausf_context.CheckIfAusfUeContextExists(resolvedSupi) {
		t.Fatalf("unexpected AUSF UE context persisted for %s", resolvedSupi)
	}
}

func TestUeAuthPostRequestProcedure_InvalidKausfDoesNotPersistContext(t *testing.T) {
	initProducerTestContext(t)
	originalResolveUdmURL := resolveUdmURL
	originalExecuteGenerateAuthData := executeGenerateAuthData
	defer func() {
		resolveUdmURL = originalResolveUdmURL
		executeGenerateAuthData = originalExecuteGenerateAuthData
	}()

	supiOrSuci := "imsi-001010000000003"
	resolvedSupi := "imsi-001010000000004"
	resolveUdmURL = func(string) string { return "https://udm.example" }
	executeGenerateAuthData = func(_ *Nudm_UEAU.APIClient, _ string, _ models.AuthenticationInfoRequest) (*models.AuthenticationInfoResult, *http.Response, error) {
		result := models.NewAuthenticationInfoResult(models.AUTHTYPE__5_G_AKA)
		result.SetSupi(resolvedSupi)
		vector := models.Av5GHeAkaAsAuthenticationVector(models.NewAv5GHeAka(
			models.AVTYPE__5_G_HE_AKA,
			"00112233445566778899aabbccddeeff",
			"00112233445566778899aabbccddeeff",
			"00112233445566778899aabbccddeeff",
			"not-hex",
		))
		result.SetAuthenticationVector(vector)
		return result, nil, nil
	}

	response, _, problemDetails := UeAuthPostRequestProcedure(models.AuthenticationInfo{
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		SupiOrSuci:         supiOrSuci,
	})

	if response != nil {
		t.Fatalf("expected nil response, got %+v", response)
	}
	if problemDetails == nil || problemDetails.GetCause() != AV_GENERATION_PROBLEM_ERROR {
		t.Fatalf("expected AV generation problem, got %+v", problemDetails)
	}
	if ausf_context.CheckIfSuciSupiPairExists(supiOrSuci) {
		t.Fatalf("unexpected SUCI/SUPI mapping persisted for %s", supiOrSuci)
	}
	if ausf_context.CheckIfAusfUeContextExists(resolvedSupi) {
		t.Fatalf("unexpected AUSF UE context persisted for %s", resolvedSupi)
	}
}
