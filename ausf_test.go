// SPDX-FileCopyrightText: 2024 Canonical Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//
/*
 * AUSF Unit Testcases
 *
 */
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/antihax/optional"
	"github.com/omec-project/ausf/consumer"
	ausfContext "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/producer"
	"github.com/omec-project/ausf/service"
	"github.com/omec-project/openapi/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/models"
	"github.com/stretchr/testify/assert"
)

var (
	AUSFTest       = &service.AUSF{}
	nfInstanceID   = "34343-4343-43-434-343"
	subscriptionID = "46326-232353-2323"
)

func setupTest() {
	if err := os.Setenv("POD_IP", "127.0.0.1"); err != nil {
		fmt.Printf("could not set env POD_IP: %+v", err)
	}
	if err := factory.InitConfigFactory("factory/ausfcfg.yaml"); err != nil {
		fmt.Printf("could not InitConfigFactory: %+v", err)
	}
}

func TestGetUDMUri(t *testing.T) {
	t.Logf("test cases for Get UDM URI")
	callCountSearchNFInstances := 0
	callCountSendNfDiscovery := 0
	origNRFCacheSearchNFInstances := consumer.NRFCacheSearchNFInstances
	origSendNfDiscoveryToNrf := consumer.SendNfDiscoveryToNrf
	udmUri1 := "https://10.0.13.1:8090"
	udmUri2 := "https://20.20.13.1:8090"
	udmProfile1 := models.NfProfile{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId:  nfInstanceID,
		Ipv4Addresses: []string{udmUri1},
		NfType:        "UDM",
		NfStatus:      "REGISTERED",
	}
	services1 := []models.NfService{
		{
			ServiceInstanceId: "datarepository",
			ServiceName:       models.ServiceName_NUDM_UEAU,
			Versions: &[]models.NfServiceVersion{
				{
					ApiFullVersion:  "1",
					ApiVersionInUri: "versionUri",
				},
			},
			Scheme:          "https",
			NfServiceStatus: models.NfServiceStatus_REGISTERED,
			ApiPrefix:       udmUri1,
			IpEndPoints: &[]models.IpEndPoint{
				{
					Ipv4Address: "10.0.13.1",
					Transport:   models.TransportProtocol_TCP,
					Port:        8090,
				},
			},
		},
	}
	udmProfile1.NfServices = &services1
	nfInstances1 := []models.NfProfile{
		udmProfile1,
	}
	searchResult1 := models.SearchResult{
		ValidityPeriod:       7,
		NfInstances:          nfInstances1,
		NrfSupportedFeatures: "",
	}
	udmProfile2 := models.NfProfile{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId:  "9999-4343-43-434-343",
		Ipv4Addresses: []string{udmUri2},
		NfType:        "UDM",
		NfStatus:      "REGISTERED",
	}
	services2 := []models.NfService{
		{
			ServiceInstanceId: "datarepository",
			ServiceName:       models.ServiceName_NUDM_UEAU,
			Versions: &[]models.NfServiceVersion{
				{
					ApiFullVersion:  "1",
					ApiVersionInUri: "versionUri",
				},
			},
			Scheme:          "https",
			NfServiceStatus: models.NfServiceStatus_REGISTERED,
			ApiPrefix:       udmUri2,
			IpEndPoints: &[]models.IpEndPoint{
				{
					Ipv4Address: "20.20.13.1",
					Transport:   models.TransportProtocol_TCP,
					Port:        8090,
				},
			},
		},
	}
	udmProfile2.NfServices = &services2
	nfInstances2 := []models.NfProfile{
		udmProfile2,
	}
	searchResult2 := models.SearchResult{
		ValidityPeriod:       7,
		NfInstances:          nfInstances2,
		NrfSupportedFeatures: "",
	}
	defer func() {
		consumer.NRFCacheSearchNFInstances = origNRFCacheSearchNFInstances
		consumer.SendNfDiscoveryToNrf = origSendNfDiscoveryToNrf
	}()
	consumer.NRFCacheSearchNFInstances = func(nrfUri string, targetNfType, requestNfType models.NfType, param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (models.SearchResult, error) {
		t.Logf("test SearchNFInstance called")
		callCountSearchNFInstances++
		return searchResult1, nil
	}
	consumer.SendNfDiscoveryToNrf = func(nrfUri string, targetNfType, requestNfType models.NfType, param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (models.SearchResult, error) {
		t.Logf("test SendNfDiscoveryToNrf called")
		callCountSendNfDiscovery++
		return searchResult2, nil
	}

	parameters := []struct {
		testName                           string
		result                             string
		udmUri                             string
		inputEnableNrfCaching              bool
		expectedCallCountSearchNFInstances int
		expectedCallCountSendNfDiscovery   int
	}{
		{
			"NRF caching is enabled request is sent to discover UDM",
			"UDM URI is retrieved from NRF cache",
			"https://10.0.13.1:8090",
			true,
			1,
			0,
		},
		{
			"NRF caching is disabled request is sent to discover UDM",
			"UDM URI is retrieved from NRF trough the NF discovery process",
			"https://20.20.13.1:8090",
			false,
			0,
			1,
		},
	}
	for i := range parameters {
		t.Run(fmt.Sprintf("NRF caching is [%v]", parameters[i].inputEnableNrfCaching), func(t *testing.T) {
			ausfContext.GetSelf().EnableNrfCaching = parameters[i].inputEnableNrfCaching
			udm_uri := producer.GetUdmUrl(ausfContext.GetSelf().NrfUri)
			assert.Equal(t, parameters[i].expectedCallCountSearchNFInstances, callCountSearchNFInstances, "NF instance is searched in the cache.")
			assert.Equal(t, parameters[i].expectedCallCountSendNfDiscovery, callCountSendNfDiscovery, "NF discovery request is sent to NRF.")
			assert.Equal(t, parameters[i].udmUri, udm_uri, "UDR Uri is set.")
			callCountSendNfDiscovery = 0
			callCountSearchNFInstances = 0
		})
	}
}

func TestCreateSubscriptionSuccess(t *testing.T) {
	t.Logf("test cases for CreateSubscription")
	udmProfile := models.NfProfile{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId: nfInstanceID,
		NfType:       "UDM",
		NfStatus:     "REGISTERED",
	}
	nfInstances := []models.NfProfile{
		udmProfile,
	}
	searchResult := models.SearchResult{
		ValidityPeriod:       7,
		NfInstances:          nfInstances,
		NrfSupportedFeatures: "",
	}
	stringReader := strings.NewReader("successful!")
	stringReadCloser := io.NopCloser(stringReader)
	httpResponse := http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		ProtoMinor: 0,
		Body:       stringReadCloser,
	}
	callCountSendCreateSubscription := 0
	origStoreApiSearchNFInstances := consumer.StoreApiSearchNFInstances
	origCreateSubscription := consumer.CreateSubscription
	defer func() {
		consumer.StoreApiSearchNFInstances = origStoreApiSearchNFInstances
		consumer.CreateSubscription = origCreateSubscription
	}()
	consumer.StoreApiSearchNFInstances = func(*Nnrf_NFDiscovery.NFInstancesStoreApiService, context.Context, models.NfType, models.NfType, *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (models.SearchResult, *http.Response, error) {
		t.Logf("test SearchNFInstances called")
		return searchResult, &httpResponse, nil
	}
	consumer.CreateSubscription = func(nrfUri string, nrfSubscriptionData models.NrfSubscriptionData) (nrfSubData models.NrfSubscriptionData, problemDetails *models.ProblemDetails, err error) {
		t.Logf("test SendCreateSubscription called")
		callCountSendCreateSubscription++
		return models.NrfSubscriptionData{
			NfStatusNotificationUri: "https://:0/nausf-callback/v1/nf-status-notify",
			ReqNfType:               "AUSF",
			SubscriptionId:          subscriptionID,
		}, nil, nil
	}
	// NRF caching is disabled
	ausfContext.GetSelf().EnableNrfCaching = false
	param := Nnrf_NFDiscovery.SearchNFInstancesParamOpts{
		ServiceNames: optional.NewInterface([]models.ServiceName{models.ServiceName_NUDM_UEAU}),
	}
	parameters := []struct {
		expectedError                           error
		testName                                string
		result                                  string
		nfInstanceId                            string
		subscriptionId                          string
		expectedCallCountSendCreateSubscription int
	}{
		{
			nil,
			"NF instances are found in Store Api subscription is not created for NFInstanceID yet",
			"Subscription is created",
			nfInstanceID,
			subscriptionID,
			1,
		},
		{
			nil,
			"NF instances are found in Store Api subscription is already created for NFInstanceID",
			"Subscription is not created again",
			nfInstanceID,
			subscriptionID,
			0,
		},
	}
	for i := range parameters {
		t.Run(fmt.Sprintf("CreateSubscription testname %v result %v", parameters[i].testName, parameters[i].result), func(t *testing.T) {
			_, err := consumer.SendNfDiscoveryToNrf("testNRFUri", "UDM", "AUSF", &param)
			val, _ := ausfContext.GetSelf().NfStatusSubscriptions.Load(parameters[i].nfInstanceId)
			assert.Equal(t, val, parameters[i].subscriptionId, "Correct Subscription ID is not stored in the AUSF context.")
			assert.Equal(t, parameters[i].expectedError, err, "SendNfDiscoveryToNrf is failed.")
			// Subscription is created.
			assert.Equal(t, parameters[i].expectedCallCountSendCreateSubscription, callCountSendCreateSubscription, "Subscription is not created for NF instance.")
			callCountSendCreateSubscription = 0
		})
	}
}

func TestCreateSubscriptionFail(t *testing.T) {
	t.Logf("test cases for CreateSubscription")
	udmProfile := models.NfProfile{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId: "84343-4343-43-434-343",
		NfType:       "UDM",
		NfStatus:     "REGISTERED",
	}
	nfInstances := []models.NfProfile{
		udmProfile,
	}
	searchResult := models.SearchResult{
		ValidityPeriod:       7,
		NfInstances:          nfInstances,
		NrfSupportedFeatures: "",
	}
	emptySearchResult := models.SearchResult{}
	nrfSubscriptionData := models.NrfSubscriptionData{
		NfStatusNotificationUri: "https://:0/nausf-callback/v1/nf-status-notify",
		ReqNfType:               "AUSF",
		SubscriptionId:          "",
	}
	emptyNrfSubscriptionData := models.NrfSubscriptionData{}
	stringReader := strings.NewReader("successful!")
	stringReadCloser := io.NopCloser(stringReader)
	httpResponseTemporaryDirect := http.Response{
		Status:     "307 Temporary Direct",
		StatusCode: 307,
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		ProtoMinor: 0,
		Body:       stringReadCloser,
	}
	httpResponseSuccess := http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		ProtoMinor: 0,
		Body:       stringReadCloser,
	}
	serverErrorProblem := models.ProblemDetails{
		Status: http.StatusInternalServerError,
		Cause:  "Server Error",
		Detail: "",
	}
	callCountSendCreateSubscription := 0
	origStoreApiSearchNFInstances := consumer.StoreApiSearchNFInstances
	origCreateSubscription := consumer.CreateSubscription
	defer func() {
		consumer.StoreApiSearchNFInstances = origStoreApiSearchNFInstances
		consumer.CreateSubscription = origCreateSubscription
	}()
	// NRF caching is disabled
	ausfContext.GetSelf().EnableNrfCaching = false
	param := Nnrf_NFDiscovery.SearchNFInstancesParamOpts{
		ServiceNames: optional.NewInterface([]models.ServiceName{models.ServiceName_NUDM_UEAU}),
	}
	parameters := []struct {
		httpResponse                            http.Response
		expectedSubscriptionId                  any
		subscriptionError                       error
		expectedError                           error
		subscriptionProblem                     *models.ProblemDetails
		nrfSubscriptionData                     models.NrfSubscriptionData
		searchResult                            models.SearchResult
		testName                                string
		result                                  string
		expectedCallCountSendCreateSubscription int
	}{
		{
			httpResponseTemporaryDirect,
			nil,
			nil,
			errors.New("temporary redirect for non NRF consumer"),
			nil,
			emptyNrfSubscriptionData,
			emptySearchResult,
			"Store Api returns HTTP code 307",
			"Subscription is not created",
			0,
		},
		{
			httpResponseSuccess,
			"",
			nil,
			nil,
			&serverErrorProblem,
			emptyNrfSubscriptionData,
			searchResult,
			"NF instances are found in Store Api subscription but create subscription reports problem",
			"Subscription request is sent but problem is reported",
			1,
		},
		{
			httpResponseSuccess,
			"",
			errors.New("SendCreateSubscription request failed"),
			errors.New("SendCreateSubscription request failed"),
			nil,
			emptyNrfSubscriptionData,
			searchResult,
			"NF instances are found in Store Api subscription but create subscription reports error",
			"Subscription request is sent but error is reported",
			1,
		},
		{
			httpResponseSuccess,
			"",
			nil,
			nil,
			nil,
			nrfSubscriptionData,
			searchResult,
			"NF instances are found in Store Api subscription subscription is created but nrfSubData does not have Subscription ID",
			"SubscriptionId is not stored in NfStatusSubscriptions",
			1,
		},
	}
	for i := range parameters {
		t.Run(fmt.Sprintf("CreateSubscription testname %v result %v", parameters[i].testName, parameters[i].result), func(t *testing.T) {
			consumer.StoreApiSearchNFInstances = func(*Nnrf_NFDiscovery.NFInstancesStoreApiService, context.Context, models.NfType, models.NfType, *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (models.SearchResult, *http.Response, error) {
				t.Logf("test SearchNFInstances called")
				return parameters[i].searchResult, &parameters[i].httpResponse, nil
			}

			consumer.CreateSubscription = func(nrfUri string, nrfSubscriptionData models.NrfSubscriptionData) (nrfSubData models.NrfSubscriptionData, problemDetails *models.ProblemDetails, err error) {
				t.Logf("test SendCreateSubscription called")
				callCountSendCreateSubscription++
				return parameters[i].nrfSubscriptionData, parameters[i].subscriptionProblem, parameters[i].subscriptionError
			}
			_, err := consumer.SendNfDiscoveryToNrf("testNRFUri", "UDM", "AUSF", &param)
			val, _ := ausfContext.GetSelf().NfStatusSubscriptions.Load(udmProfile.NfInstanceId)
			assert.Equal(t, val, parameters[i].expectedSubscriptionId, "Correct Subscription ID is not stored in the AUSF context.")
			assert.Equal(t, parameters[i].expectedError, err, "SendNfDiscoveryToNrf is failed.")
			assert.Equal(t, parameters[i].expectedCallCountSendCreateSubscription, callCountSendCreateSubscription, "Subscription is not created for NF instance.")
			callCountSendCreateSubscription = 0
			ausfContext.GetSelf().NfStatusSubscriptions.Delete(udmProfile.NfInstanceId)
		})
	}
}

func TestNfSubscriptionStatusNotify(t *testing.T) {
	t.Logf("test cases for NfSubscriptionStatusNotify")
	callCountSendRemoveSubscription := 0
	callCountNRFCacheRemoveNfProfileFromNrfCache := 0
	origSendRemoveSubscription := consumer.SendRemoveSubscription
	origNRFCacheRemoveNfProfileFromNrfCache := producer.NRFCacheRemoveNfProfileFromNrfCache
	defer func() {
		consumer.SendRemoveSubscription = origSendRemoveSubscription
		producer.NRFCacheRemoveNfProfileFromNrfCache = origNRFCacheRemoveNfProfileFromNrfCache
	}()
	consumer.SendRemoveSubscription = func(subscriptionId string) (problemDetails *models.ProblemDetails, err error) {
		t.Logf("test SendRemoveSubscription called")
		callCountSendRemoveSubscription++
		return nil, nil
	}
	producer.NRFCacheRemoveNfProfileFromNrfCache = func(nfInstanceId string) bool {
		t.Logf("test NRFCacheRemoveNfProfileFromNrfCache called")
		callCountNRFCacheRemoveNfProfileFromNrfCache++
		return true
	}
	udmProfile := models.NfProfileNotificationData{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId: nfInstanceID,
		NfType:       "UDM",
		NfStatus:     "DEREGISTERED",
	}
	badRequestProblem := models.ProblemDetails{
		Status: http.StatusBadRequest,
		Cause:  "MANDATORY_IE_MISSING",
		Detail: "Missing IE [Event]/[NfInstanceUri] in NotificationData",
	}
	parameters := []struct {
		expectedProblem                                      *models.ProblemDetails
		testName                                             string
		result                                               string
		nfInstanceId                                         string
		nfInstanceIdForSubscription                          string
		subscriptionID                                       string
		notificationEventType                                string
		expectedCallCountSendRemoveSubscription              int
		expectedCallCountNRFCacheRemoveNfProfileFromNrfCache int
		enableNrfCaching                                     bool
	}{
		{
			nil,
			"Notification event type DEREGISTERED NRF caching is enabled",
			"NF profile removed from cache and subscription is removed",
			nfInstanceID,
			nfInstanceID,
			subscriptionID,
			"NF_DEREGISTERED",
			1,
			1,
			true,
		},
		{
			nil,
			"Notification event type DEREGISTERED NRF caching is enabled Subscription is not found",
			"NF profile removed from cache and subscription is not removed",
			nfInstanceID,
			"",
			"",
			"NF_DEREGISTERED",
			0,
			1,
			true,
		},
		{
			nil,
			"Notification event type DEREGISTERED NRF caching is disabled",
			"NF profile is not removed from cache and subscription is removed",
			nfInstanceID,
			nfInstanceID,
			subscriptionID,
			"NF_DEREGISTERED",
			1,
			0,
			false,
		},
		{
			nil,
			"Notification event type REGISTERED NRF caching is enabled",
			"NF profile is not removed from cache and subscription is not removed",
			nfInstanceID,
			nfInstanceID,
			subscriptionID,
			"NF_REGISTERED",
			0,
			0,
			true,
		},
		{
			nil,
			"Notification event type DEREGISTERED NRF caching is enabled NfInstanceUri in notificationData is different",
			"NF profile removed from cache and subscription is not removed",
			nfInstanceID,
			nfInstanceID,
			subscriptionID,
			"NF_DEREGISTERED",
			1,
			1,
			true,
		},
		{
			&badRequestProblem,
			"Notification event type DEREGISTERED NRF caching is enabled NfInstanceUri in notificationData is empty",
			"Return StatusBadRequest with cause MANDATORY_IE_MISSING",
			"",
			"",
			subscriptionID,
			"NF_DEREGISTERED",
			0,
			0,
			true,
		},
		{
			&badRequestProblem,
			"Notification event type empty NRF caching is enabled",
			"Return StatusBadRequest with cause MANDATORY_IE_MISSING",
			nfInstanceID,
			nfInstanceID,
			subscriptionID,
			"",
			0,
			0,
			true,
		},
	}
	for i := range parameters {
		t.Run(fmt.Sprintf("NfSubscriptionStatusNotify testname %v result %v", parameters[i].testName, parameters[i].result), func(t *testing.T) {
			ausfContext.GetSelf().EnableNrfCaching = parameters[i].enableNrfCaching
			ausfContext.GetSelf().NfStatusSubscriptions.Store(parameters[i].nfInstanceIdForSubscription, parameters[i].subscriptionID)
			notificationData := models.NotificationData{
				Event:          models.NotificationEventType(parameters[i].notificationEventType),
				NfInstanceUri:  parameters[i].nfInstanceId,
				NfProfile:      &udmProfile,
				ProfileChanges: []models.ChangeItem{},
			}
			err := producer.NfSubscriptionStatusNotifyProcedure(notificationData)
			assert.Equal(t, parameters[i].expectedProblem, err, "NfSubscriptionStatusNotifyProcedure is failed.")
			// Subscription is removed.
			assert.Equal(t, parameters[i].expectedCallCountSendRemoveSubscription, callCountSendRemoveSubscription, "Subscription is not removed.")
			// NF Profile is removed from NRF cache.
			assert.Equal(t, parameters[i].expectedCallCountNRFCacheRemoveNfProfileFromNrfCache, callCountNRFCacheRemoveNfProfileFromNrfCache, "NF Profile is not removed from NRF cache.")
			callCountSendRemoveSubscription = 0
			callCountNRFCacheRemoveNfProfileFromNrfCache = 0
			ausfContext.GetSelf().NfStatusSubscriptions.Delete(parameters[i].nfInstanceIdForSubscription)
		})
	}
}

func TestMain(m *testing.M) {
	setupTest()
	exitVal := m.Run()
	os.Exit(exitVal)
}
