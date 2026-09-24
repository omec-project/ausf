// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"testing"

	ausfctx "github.com/omec-project/ausf/context"
	"github.com/omec-project/openapi/v2/models"
)

func TestGetNfProfile_WhenContextIsNil_ThenReturnsError(t *testing.T) {
	_, err := getNfProfile(nil, nil)
	if err == nil {
		t.Fatal("expected an error when ausf context is nil, got nil")
	}
}

func TestGetNfProfile_WhenNfServicesArePresent_ThenNfServiceListIsKeyedByServiceInstanceId(t *testing.T) {
	nfService := models.NFService{
		ServiceInstanceId: "some-instance-id",
		ServiceName:       models.SERVICENAME_NAUSF_AUTH,
		Scheme:            models.URISCHEME_HTTPS,
		NfServiceStatus:   models.NFSERVICESTATUS_REGISTERED,
	}
	context := &ausfctx.AUSFContext{
		NfId:         "test-nf-id",
		RegisterIPv4: "127.0.0.1",
		NfService: map[models.ServiceName]models.NFService{
			models.SERVICENAME_NAUSF_AUTH: nfService,
		},
	}

	profile, err := getNfProfile(context, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	nfServiceList := profile.GetNfServiceList()
	if len(nfServiceList) != 1 {
		t.Fatalf("expected 1 entry in NfServiceList, got %d", len(nfServiceList))
	}
	// per TS 29.510, nfServiceList is keyed by serviceInstanceId
	got, ok := nfServiceList[nfService.ServiceInstanceId]
	if !ok {
		t.Fatalf("expected NfServiceList to be keyed by service instance id %q, got keys: %v", nfService.ServiceInstanceId, nfServiceList)
	}
	if got.ServiceName != nfService.ServiceName {
		t.Errorf("expected service name %q, got %q", nfService.ServiceName, got.ServiceName)
	}

	nfServices := profile.GetNfServices()
	if len(nfServices) != 1 || nfServices[0].ServiceName != models.SERVICENAME_NAUSF_AUTH {
		t.Errorf("expected NfServices to contain the registered service, got: %+v", nfServices)
	}
}

func TestGetNfProfile_WhenNoNfServices_ThenNfServiceListIsNotSet(t *testing.T) {
	context := &ausfctx.AUSFContext{
		NfId:         "test-nf-id",
		RegisterIPv4: "127.0.0.1",
	}

	profile, err := getNfProfile(context, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if profile.HasNfServiceList() {
		t.Errorf("expected NfServiceList to be unset when there are no NF services")
	}
}
