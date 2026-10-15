// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"testing"

	ausfctx "github.com/omec-project/ausf/context"
	"github.com/omec-project/openapi/v2/models"
)

func TestBuildSubscriptionData_SetsNotificationUriCondAndReqNfType(t *testing.T) {
	ausfSelf := &ausfctx.AUSFContext{
		UriScheme:    models.URISCHEME_HTTPS,
		RegisterIPv4: testRegisterIPv4,
		SBIPort:      29509,
	}
	const nfInstanceID = "test-nf-instance-id"

	got := buildSubscriptionData(ausfSelf, nfInstanceID, models.NFTYPE_AMF)

	wantUri := ausfSelf.GetIPv4Uri() + "/nausf-callback/v1/nf-status-notify"
	if got.GetNfStatusNotificationUri() != wantUri {
		t.Errorf("expected NfStatusNotificationUri %q, got %q", wantUri, got.GetNfStatusNotificationUri())
	}

	subscrCond := got.GetSubscrCond()
	if subscrCond.NfInstanceIdCond == nil {
		t.Fatal("expected SubscrCond to wrap a NfInstanceIdCond, got nil")
	}
	if gotInstanceID := subscrCond.NfInstanceIdCond.GetNfInstanceId(); gotInstanceID != nfInstanceID {
		t.Errorf("expected NfInstanceIdCond.NfInstanceId %q, got %q", nfInstanceID, gotInstanceID)
	}

	if got.GetReqNfType() != models.NFTYPE_AMF {
		t.Errorf("expected ReqNfType %q, got %q", models.NFTYPE_AMF, got.GetReqNfType())
	}
}
