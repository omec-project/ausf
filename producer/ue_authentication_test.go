// Copyright (c) 2026 Intel Corporation
// Copyright 2024 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0

package producer_test

import (
	"fmt"
	"net/http"
	"testing"

	ausf_context "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/producer"
	"github.com/omec-project/openapi/v2/models"
)

func TestGenerateRandomNumber(t *testing.T) {
	value, err := producer.GenerateRandomNumber()
	if err != nil {
		t.Fatalf("GenerateRandomNumber() failed: %s", err)
	}

	t.Logf("random number: %d", value)
}

func TestAuth5gAkaComfirmRequestProcedureReturnsNotFoundForMissingAuthContext(t *testing.T) {
	response, problemDetails := producer.Auth5gAkaComfirmRequestProcedure(models.ConfirmationData{}, fmt.Sprintf("missing-%s", t.Name()))
	if response != nil {
		t.Fatalf("expected nil response, got %+v", response)
	}
	if problemDetails == nil {
		t.Fatal("expected problem details")
	}
	if got := problemDetails.GetStatus(); got != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, got)
	}
}

func TestAuth5gAkaComfirmRequestProcedureReturnsNotFoundForMissingUeContext(t *testing.T) {
	confirmationID := fmt.Sprintf("confirmation-%s", t.Name())
	supi := fmt.Sprintf("imsi-%s", t.Name())
	ausf_context.AddSuciSupiPairToMap(confirmationID, supi)
	defer ausf_context.RemoveSuciSupiPairFromMap(confirmationID)

	response, problemDetails := producer.Auth5gAkaComfirmRequestProcedure(models.ConfirmationData{}, confirmationID)
	if response != nil {
		t.Fatalf("expected nil response, got %+v", response)
	}
	if problemDetails == nil {
		t.Fatal("expected problem details")
	}
	if got := problemDetails.GetStatus(); got != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, got)
	}
}
