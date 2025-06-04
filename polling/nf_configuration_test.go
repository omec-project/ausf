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
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/nrfregistration"
	"github.com/omec-project/openapi/models"
	"github.com/stretchr/testify/assert"
)

func TestHandlePolledPlmnConfig_ConfigChanged_CallsNRFRegistration(t *testing.T) {
	testCases := []struct {
		name          string
		newPlmnConfig []models.PlmnId
	}{
		{
			name:          "ConfigChangedOneElement",
			newPlmnConfig: []models.PlmnId{{Mcc: "001", Mnc: "02"}},
		},
		{
			name:          "ConfigChangedTwoElements",
			newPlmnConfig: []models.PlmnId{{Mcc: "001", Mnc: "02"}, {Mcc: "022", Mnc: "02"}},
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
				PlmnList: []models.PlmnId{{Mcc: "001", Mnc: "01"}},
			}

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

func TestHandlePolledPlmnConfig_ConfigDidNotChanged_DoesNotCallNRFRegistration(t *testing.T) {
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
				PlmnList: tc.newPlmnConfig,
			}

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

func TestFetchPlmnConfig(t *testing.T) {
	validPlmnList := []models.PlmnId{
		{Mcc: "001", Mnc: "01"},
		{Mcc: "002", Mnc: "02"},
	}
	validJson, _ := json.Marshal(validPlmnList)

	tests := []struct {
		name           string
		statusCode     int
		contentType    string
		responseBody   string
		expectedError  string
		expectedResult []models.PlmnId
	}{
		{
			name:           "200 OK with valid JSON",
			statusCode:     http.StatusOK,
			contentType:    "application/json",
			responseBody:   string(validJson),
			expectedError:  "",
			expectedResult: validPlmnList,
		},
		{
			name:          "200 OK with invalid Content-Type",
			statusCode:    http.StatusOK,
			contentType:   "text/plain",
			responseBody:  string(validJson),
			expectedError: "unexpected Content-Type: got text/plain, want application/json",
		},
		{
			name:          "400 Bad Request",
			statusCode:    http.StatusBadRequest,
			contentType:   "application/json",
			responseBody:  "",
			expectedError: "server returned 400 error code",
		},
		{
			name:          "500 Internal Server Error",
			statusCode:    http.StatusInternalServerError,
			contentType:   "application/json",
			responseBody:  "",
			expectedError: "server returned 500 error code",
		},
		{
			name:          "Unexpected Status Code 418",
			statusCode:    418,
			contentType:   "application/json",
			responseBody:  "",
			expectedError: "unexpected status code: 418",
		},
		{
			name:          "200 OK with invalid JSON",
			statusCode:    http.StatusOK,
			contentType:   "application/json",
			responseBody:  "{invalid-json}",
			expectedError: "failed to parse JSON response:",
		},
	}

	for _, tc := range tests {

		t.Run(tc.name, func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				accept := r.Header.Get("Accept")
				assert.Equal(t, "application/json", accept)

				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.responseBody))
			}
			server := httptest.NewServer(http.HandlerFunc(handler))
			defer server.Close()

			factory.AusfConfig = factory.Config{
				Configuration: &factory.Configuration{
					WebuiUri: server.URL,
				},
			}
			fetchedConfig, err := fetchPlmnConfig()

			if tc.expectedError == "" {
				if err != nil {
					t.Errorf("expected no error, got `%v`", err)
				}
				if !reflect.DeepEqual(tc.expectedResult, fetchedConfig) {
					t.Errorf("error in fetched config: expected `%v`, got `%v`", tc.expectedResult, fetchedConfig)
				}
			} else {
				if err == nil {
					t.Errorf("expected error `%v`, got nil", tc.expectedError)
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("expected error `%v`, got `%v`", tc.expectedError, err)
				}
			}

			factory.AusfConfig.Configuration = nil
		})
	}
}
