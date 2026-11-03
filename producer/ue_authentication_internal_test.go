// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	ausf_context "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/openapi/v2/Nudm_UEAU"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
)

const (
	testUdmUrl         = "https://udm.example"
	testServingNetwork = "5G:mnc001.mcc001.3gppnetwork.org"
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
	resolveUdmURL = func(string) string { return testUdmUrl }
	executeGenerateAuthData = func(_ *Nudm_UEAU.APIClient, _ string, _ models.AuthenticationInfoRequest) (*models.AuthenticationInfoResult, *http.Response, error) {
		result := models.NewAuthenticationInfoResult(models.AuthType("UNSUPPORTED"))
		result.SetSupi(resolvedSupi)
		return result, nil, nil
	}

	response, _, problemDetails := UeAuthPostRequestProcedure(models.AuthenticationInfo{
		ServingNetworkName: testServingNetwork,
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
	resolveUdmURL = func(string) string { return testUdmUrl }
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
		ServingNetworkName: testServingNetwork,
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

func TestDeleteAuthenticationResultProcedureRemovesLocalState(t *testing.T) {
	initProducerTestContext(t)
	originalExecuteDeleteAuth := executeDeleteAuth
	defer func() {
		executeDeleteAuth = originalExecuteDeleteAuth
	}()

	authCtxID := "imsi-001010000000010"
	supi := "imsi-001010000000011"
	ausf_context.AddSuciSupiPairToMap(authCtxID, supi)
	defer ausf_context.RemoveSuciSupiPairFromMap(authCtxID)
	ausf_context.AddAusfUeContextToPool(&ausf_context.AusfUeContext{
		Supi:               supi,
		ServingNetworkName: testServingNetwork,
		UdmUeauUrl:         testUdmUrl,
	})
	defer ausf_context.RemoveAusfUeContextFromPool(supi)

	called := false
	executeDeleteAuth = func(_ *Nudm_UEAU.APIClient, gotSupi, gotAuthEventID string, _ models.AuthEvent) (*http.Response, error) {
		called = true
		if gotSupi != supi || gotAuthEventID != authCtxID {
			t.Fatalf("unexpected delete auth inputs: supi=%s authEventID=%s", gotSupi, gotAuthEventID)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
	}

	problemDetails := DeleteAuthenticationResultProcedure(authCtxID, models.AUTHTYPE__5_G_AKA)
	if problemDetails != nil {
		t.Fatalf("expected no problem details, got %+v", problemDetails)
	}
	if !called {
		t.Fatal("expected delete auth call")
	}
	if ausf_context.CheckIfSuciSupiPairExists(authCtxID) {
		t.Fatalf("expected auth context mapping removed for %s", authCtxID)
	}
	if ausf_context.CheckIfAusfUeContextExists(supi) {
		t.Fatalf("expected AUSF UE context removed for %s", supi)
	}
}

func TestDeregisterAuthContextProcedureRemovesAllMatchingAuthContexts(t *testing.T) {
	initProducerTestContext(t)
	originalExecuteDeleteAuth := executeDeleteAuth
	defer func() {
		executeDeleteAuth = originalExecuteDeleteAuth
	}()

	supi := "imsi-001010000000012"
	authCtxIDs := []string{"suci-001", "suci-002"}
	for _, authCtxID := range authCtxIDs {
		ausf_context.AddSuciSupiPairToMap(authCtxID, supi)
		defer ausf_context.RemoveSuciSupiPairFromMap(authCtxID)
	}
	ausf_context.AddAusfUeContextToPool(&ausf_context.AusfUeContext{
		Supi:               supi,
		ServingNetworkName: testServingNetwork,
		UdmUeauUrl:         testUdmUrl,
		XRES:               "xres",
	})
	defer ausf_context.RemoveAusfUeContextFromPool(supi)

	deletedAuthCtxIDs := make(map[string]struct{})
	executeDeleteAuth = func(_ *Nudm_UEAU.APIClient, gotSupi, gotAuthEventID string, _ models.AuthEvent) (*http.Response, error) {
		if gotSupi != supi {
			t.Fatalf("unexpected supi %s", gotSupi)
		}
		deletedAuthCtxIDs[gotAuthEventID] = struct{}{}
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
	}

	problemDetails := DeregisterAuthContextProcedure(*models.NewDeregistrationInfo(supi))
	if problemDetails != nil {
		t.Fatalf("expected no problem details, got %+v", problemDetails)
	}
	if len(deletedAuthCtxIDs) != len(authCtxIDs) {
		t.Fatalf("expected %d delete auth calls, got %d", len(authCtxIDs), len(deletedAuthCtxIDs))
	}
	for _, authCtxID := range authCtxIDs {
		if ausf_context.CheckIfSuciSupiPairExists(authCtxID) {
			t.Fatalf("expected auth context mapping removed for %s", authCtxID)
		}
	}
	if ausf_context.CheckIfAusfUeContextExists(supi) {
		t.Fatalf("expected AUSF UE context removed for %s", supi)
	}
}

func TestInvalidateUdmCache_ClearsBothCaches(t *testing.T) {
	initProducerTestContext(t)

	ausf_context.GetSelf().UdmUeauUrl = testUdmUrl
	cachedUdmClient = Nudm_UEAU.NewAPIClient(Nudm_UEAU.NewConfiguration())
	cachedUdmClientURL = testUdmUrl
	t.Cleanup(func() {
		ausf_context.GetSelf().UdmUeauUrl = ""
		cachedUdmClient = nil
		cachedUdmClientURL = ""
	})

	invalidateUdmCache()

	if ausf_context.GetSelf().UdmUeauUrl != "" {
		t.Errorf("UdmUeauUrl not cleared: got %q", ausf_context.GetSelf().UdmUeauUrl)
	}
	if cachedUdmClient != nil {
		t.Error("cachedUdmClient not cleared")
	}
	if cachedUdmClientURL != "" {
		t.Errorf("cachedUdmClientURL not cleared: got %q", cachedUdmClientURL)
	}
}

func TestUeAuthPostRequestProcedure_MapsUdmErrors(t *testing.T) {
	initProducerTestContext(t)
	originalResolveUdmURL := resolveUdmURL
	defer func() { resolveUdmURL = originalResolveUdmURL }()

	const udmCause = "UDM_SPECIFIC_CAUSE"
	problemBody := func(cause string) string {
		if cause == "" {
			return `{"title":"udm error"}`
		}
		return `{"title":"udm error","cause":"` + cause + `"}`
	}

	tests := []struct {
		name       string
		udmStatus  int    // 0 means the UDM is unreachable
		udmBody    string // ProblemDetails JSON returned by the UDM
		wantStatus int
		wantCause  string
	}{
		{name: "transport failure", wantStatus: http.StatusGatewayTimeout, wantCause: UPSTREAM_SERVER_ERROR},
		{name: "400 without cause", udmStatus: http.StatusBadRequest, udmBody: problemBody(""), wantStatus: http.StatusBadRequest, wantCause: utils.CauseInvalidRequest},
		{name: "400 with cause", udmStatus: http.StatusBadRequest, udmBody: problemBody(udmCause), wantStatus: http.StatusBadRequest, wantCause: udmCause},
		{name: "403 without cause", udmStatus: http.StatusForbidden, udmBody: problemBody(""), wantStatus: http.StatusForbidden, wantCause: utils.CauseAuthenticationRejected},
		{name: "403 with cause", udmStatus: http.StatusForbidden, udmBody: problemBody(udmCause), wantStatus: http.StatusForbidden, wantCause: udmCause},
		{name: "404 without cause", udmStatus: http.StatusNotFound, udmBody: problemBody(""), wantStatus: http.StatusNotFound, wantCause: utils.CauseUserNotFound},
		{name: "404 with cause", udmStatus: http.StatusNotFound, udmBody: problemBody(udmCause), wantStatus: http.StatusNotFound, wantCause: udmCause},
		{name: "400 with unparsable body", udmStatus: http.StatusBadRequest, udmBody: "not json", wantStatus: http.StatusBadRequest, wantCause: utils.CauseInvalidRequest},
		{name: "500 falls back", udmStatus: http.StatusInternalServerError, udmBody: problemBody(udmCause), wantStatus: http.StatusInternalServerError, wantCause: AV_GENERATION_PROBLEM_ERROR},
		{name: "503 falls back", udmStatus: http.StatusServiceUnavailable, udmBody: problemBody(udmCause), wantStatus: http.StatusInternalServerError, wantCause: AV_GENERATION_PROBLEM_ERROR},
		{name: "401 falls back", udmStatus: http.StatusUnauthorized, udmBody: problemBody(udmCause), wantStatus: http.StatusInternalServerError, wantCause: AV_GENERATION_PROBLEM_ERROR},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			udm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(tt.udmStatus)
				if _, err := w.Write([]byte(tt.udmBody)); err != nil {
					t.Errorf("write UDM response: %v", err)
				}
			}))
			if tt.udmStatus == 0 {
				udm.Close() // connections to the closed listener are refused, so no HTTP response exists
			} else {
				defer udm.Close()
			}
			resolveUdmURL = func(string) string { return udm.URL }

			supiOrSuci := "imsi-001010000000050"
			response, _, problemDetails := UeAuthPostRequestProcedure(models.AuthenticationInfo{
				ServingNetworkName: testServingNetwork,
				SupiOrSuci:         supiOrSuci,
			})

			if response != nil {
				t.Fatalf("expected nil response, got %+v", response)
			}
			if problemDetails == nil {
				t.Fatal("expected problem details, got nil")
			}
			if problemDetails.GetStatus() != int32(tt.wantStatus) || problemDetails.GetCause() != tt.wantCause {
				t.Fatalf("got status %d cause %q, want status %d cause %q",
					problemDetails.GetStatus(), problemDetails.GetCause(), tt.wantStatus, tt.wantCause)
			}
			if ausf_context.CheckIfSuciSupiPairExists(supiOrSuci) {
				t.Fatalf("unexpected SUCI/SUPI mapping persisted for %s", supiOrSuci)
			}
		})
	}
}

func TestUeAuthPostRequestProcedure_EapAkaPrimeChallengeFramingAndMac(t *testing.T) {
	initProducerTestContext(t)
	originalResolveUdmURL := resolveUdmURL
	originalExecuteGenerateAuthData := executeGenerateAuthData
	defer func() {
		resolveUdmURL = originalResolveUdmURL
		executeGenerateAuthData = originalExecuteGenerateAuthData
	}()

	const (
		supiOrSuci = "imsi-001010000000005"
		supi       = "imsi-0555444333222111"
		// RFC 5448 Appendix C Case 1
		randHex = "81e92b6c0ee0e12ebceba8d92a99dfa5"
		autnHex = "bb52e91c747ac3ab2a5c23d15ee351d5"
		ckPrime = "0093962d0dd84aa5684b045c9edffa04"
		ikPrime = "ccfc230ca74fcc96c0a5d61164f5a76c"
		// first 256 bits of the Case 1 EMSK (TS 33.501 clause 6.1.3.1) and KSEAF = KDF(KAUSF, 0x6C, serving
		// network name) of TS 33.501 Annex A.6, computed outside the AUSF code
		wantKausf = "f861703cd775590e16c7679ea3874ada866311de290764d760cf76df647ea01c"
		wantKseaf = "d99768468fefbf0f681d70dfc3c3848af7ce043e276cd366d81ec74bce5dfbaa"
	)
	resolveUdmURL = func(string) string { return testUdmUrl }
	executeGenerateAuthData = func(_ *Nudm_UEAU.APIClient, _ string, _ models.AuthenticationInfoRequest) (*models.AuthenticationInfoResult, *http.Response, error) {
		result := models.NewAuthenticationInfoResult(models.AUTHTYPE_EAP_AKA_PRIME)
		result.SetSupi(supi)
		result.SetAuthenticationVector(models.AvEapAkaPrimeAsAuthenticationVector(models.NewAvEapAkaPrime(
			models.AVTYPE_EAP_AKA_PRIME, randHex, "0102030405060708", autnHex, ckPrime, ikPrime)))
		return result, nil, nil
	}
	defer ausf_context.RemoveSuciSupiPairFromMap(supiOrSuci)
	defer ausf_context.RemoveAusfUeContextFromPool(supi)

	response, _, problemDetails := UeAuthPostRequestProcedure(models.AuthenticationInfo{
		ServingNetworkName: testServingNetwork,
		SupiOrSuci:         supiOrSuci,
	})
	if problemDetails != nil || response == nil {
		t.Fatalf("unexpected result: response=%v problem=%+v", response, problemDetails)
	}
	authData := response.GetVar5gAuthData()
	payload, ok := authData.GetActualInstance().(*string)
	if !ok || payload == nil {
		t.Fatalf("expected EAP payload string, got %T", authData.GetActualInstance())
	}
	wire, err := base64.StdEncoding.DecodeString(*payload)
	if err != nil {
		t.Fatalf("decode EAP payload: %v", err)
	}
	pkt, err := EapDecode(wire)
	if err != nil {
		t.Fatalf("EapDecode() error = %v", err)
	}
	if pkt.Code != EapCodeForRequest || pkt.Type != EapTypeEapAkaPrime {
		t.Fatalf("got Code %d Type %d, want Request / EAP-AKA'", pkt.Code, pkt.Type)
	}
	if !bytes.Equal(pkt.TypeData[:eapAkaHeaderSize], akaChallengeHeader) {
		t.Fatalf("AKA header = %x, want %x", pkt.TypeData[:eapAkaHeaderSize], akaChallengeHeader)
	}

	// walk the attributes: AT_RAND, AT_AUTN, AT_MAC, AT_KDF, AT_KDF_INPUT, each 4-byte aligned
	wantOrder := []byte{
		ausf_context.AT_RAND_ATTRIBUTE, ausf_context.AT_AUTN_ATTRIBUTE, ausf_context.AT_MAC_ATTRIBUTE,
		ausf_context.AT_KDF_ATTRIBUTE, ausf_context.AT_KDF_INPUT_ATTRIBUTE,
	}
	attrs := pkt.TypeData[eapAkaHeaderSize:]
	macOffset := -1
	var got []byte
	for i := 0; i < len(attrs); {
		if i+2 > len(attrs) || attrs[i+1] == 0 || i+int(attrs[i+1])*4 > len(attrs) {
			t.Fatalf("malformed attribute at offset %d: %x", i, attrs)
		}
		got = append(got, attrs[i])
		if attrs[i] == ausf_context.AT_MAC_ATTRIBUTE {
			macOffset = eapAkaHeaderSize + i
		}
		i += int(attrs[i+1]) * 4
	}
	if !bytes.Equal(got, wantOrder) {
		t.Fatalf("attribute order = %v, want %v", got, wantOrder)
	}

	ueCtx := ausf_context.GetAusfUeContext(supi)
	if ueCtx == nil {
		t.Fatalf("no AUSF UE context stored for %s", supi)
	}
	if len(ueCtx.Kausf) != 64 || ueCtx.Kausf != wantKausf {
		t.Fatalf("Kausf = %q, want 256-bit %q", ueCtx.Kausf, wantKausf)
	}
	if ueCtx.Kseaf != wantKseaf {
		t.Fatalf("Kseaf = %q, want %q", ueCtx.Kseaf, wantKseaf)
	}

	// AT_MAC covers the whole EAP packet with the MAC value zeroed (RFC 4187 section 10.15)
	zeroed := bytes.Clone(wire)
	macValueStart := 5 + macOffset + 4 // EAP header (4) + Type (1) + TypeData offset + AT_MAC header
	gotMac := bytes.Clone(zeroed[macValueStart : macValueStart+16])
	clear(zeroed[macValueStart : macValueStart+16])
	h := hmac.New(sha256.New, []byte(ueCtx.K_aut))
	h.Write(zeroed)
	if wantMac := h.Sum(nil)[:16]; !bytes.Equal(gotMac, wantMac) {
		t.Fatalf("AT_MAC = %x, want %x", gotMac, wantMac)
	}
}

func TestUeAuthPostRequestProcedure_EapAkaPrimeInvalidKeysDoNotPersistContext(t *testing.T) {
	// Built at runtime so secret scanners don't flag a key-like literal.
	goodKey := string(bytes.Repeat([]byte("ab"), 16))
	tests := []struct {
		name, ckPrime, ikPrime string
	}{
		{name: "IK' not hex", ckPrime: goodKey, ikPrime: "not-hex"},
		{name: "CK' not hex", ckPrime: "not-hex", ikPrime: goodKey},
		{name: "IK' too short", ckPrime: goodKey, ikPrime: "0011"},
		{name: "CK' too long", ckPrime: goodKey + "00", ikPrime: goodKey},
		{name: "empty keys", ckPrime: "", ikPrime: ""},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initProducerTestContext(t)
			originalResolveUdmURL := resolveUdmURL
			originalExecuteGenerateAuthData := executeGenerateAuthData
			defer func() {
				resolveUdmURL = originalResolveUdmURL
				executeGenerateAuthData = originalExecuteGenerateAuthData
			}()

			supiOrSuci := fmt.Sprintf("imsi-00101000000100%d", i)
			supi := fmt.Sprintf("imsi-00101000000200%d", i)
			resolveUdmURL = func(string) string { return testUdmUrl }
			executeGenerateAuthData = func(_ *Nudm_UEAU.APIClient, _ string, _ models.AuthenticationInfoRequest) (*models.AuthenticationInfoResult, *http.Response, error) {
				result := models.NewAuthenticationInfoResult(models.AUTHTYPE_EAP_AKA_PRIME)
				result.SetSupi(supi)
				result.SetAuthenticationVector(models.AvEapAkaPrimeAsAuthenticationVector(models.NewAvEapAkaPrime(
					models.AVTYPE_EAP_AKA_PRIME, goodKey, "0102030405060708", goodKey, tt.ckPrime, tt.ikPrime)))
				return result, nil, nil
			}

			response, _, problemDetails := UeAuthPostRequestProcedure(models.AuthenticationInfo{
				ServingNetworkName: testServingNetwork,
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
			if ausf_context.CheckIfAusfUeContextExists(supi) {
				t.Fatalf("unexpected AUSF UE context persisted for %s", supi)
			}
		})
	}
}
