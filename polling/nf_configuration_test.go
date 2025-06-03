// SPDX-FileCopyrightText: 2025 Canonical Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//
/*
 * NF Polling Unit Tests
 *
 */

package polling

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/nrfregistration"
	"github.com/omec-project/openapi/models"
)

func TestHandlePolledPlmnConfig_UpdateConfig(t *testing.T) {

	testCases := []struct {
		name          string
		newPlmnConfig []models.PlmnId
	}{
		{
			name:          "ConfigChanged",
			newPlmnConfig: []models.PlmnId{{Mcc: "001", Mnc: "02"}},
		},
		{
			name:          "EmptyConfig",
			newPlmnConfig: []models.PlmnId{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			originalFunction := nrfregistration.HandleNewConfig
			called := false
			nrfregistration.HandleNewConfig = func(newPlmnConfig []models.PlmnId) { called = true }
			defer func() { nrfregistration.HandleNewConfig = originalFunction }()

			context := &context.AUSFContext{
				PlmnList: []models.PlmnId{{Mcc: "001", Mnc: "01"}}}

			handlePolledPlmnConfig(context, tc.newPlmnConfig)

			if !reflect.DeepEqual(context.PlmnList, tc.newPlmnConfig) {
				t.Errorf("Expected PLMN config to be updated to %v, got %v", tc.newPlmnConfig, context.PlmnList)
			}
			if !called {
				t.Error("Expected nrfregistration.HandleNewConfig to be called")
			}
		})
	}
}

func TestHandlePolledPlmnConfig_ConfigDidNotChanged(t *testing.T) {

	testCases := []struct {
		name          string
		newPlmnConfig []models.PlmnId
	}{
		{
			name:          "SameConfig",
			newPlmnConfig: []models.PlmnId{{Mcc: "001", Mnc: "02"}},
		},
		{
			name:          "EmptyConfig",
			newPlmnConfig: []models.PlmnId{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			originalFunction := nrfregistration.HandleNewConfig
			called := false
			nrfregistration.HandleNewConfig = func(newPlmnConfig []models.PlmnId) { called = true }
			defer func() { nrfregistration.HandleNewConfig = originalFunction }()

			context := &context.AUSFContext{
				PlmnList: tc.newPlmnConfig}

			handlePolledPlmnConfig(context, tc.newPlmnConfig)

			if !reflect.DeepEqual(context.PlmnList, tc.newPlmnConfig) {
				t.Errorf("Expected PLMN list to remain unchanged, got %v", context.PlmnList)
			}

			if called {
				t.Error("Expected nrfregistration.HandleNewConfig not to be called")
			}
		})
	}
}

func TestFetchPlmnConfig_Success(t *testing.T) {
	expectedPlmnConfig := []models.PlmnId{{Mcc: "001", Mnc: "01"}}
	handler := func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(expectedPlmnConfig)
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	factory.AusfConfig = factory.Config{
		Configuration: &factory.Configuration{
			WebuiUri: server.URL,
		},
	}

	fetchedConfig, err := fetchPlmnConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !reflect.DeepEqual(fetchedConfig, expectedPlmnConfig) {
		t.Errorf("expected %v, got %v", expectedPlmnConfig, fetchedConfig)
	}
}

func TestFetchPlmnConfig_BadJSON(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "not-json")
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	factory.AusfConfig = factory.Config{
		Configuration: &factory.Configuration{
			WebuiUri: server.URL,
		},
	}

	_, err := fetchPlmnConfig()
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}
