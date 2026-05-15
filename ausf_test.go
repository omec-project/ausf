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
	"reflect"
	"strings"
	"testing"

	"github.com/omec-project/ausf/consumer"
	ausfContext "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/producer"
	"github.com/omec-project/ausf/service"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/v2/models"
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
	udmProfile1 := models.NFProfileDiscovery{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId:  nfInstanceID,
		Ipv4Addresses: []string{udmUri1},
		NfType:        "UDM",
		NfStatus:      "REGISTERED",
	}
	services1 := []models.NFService{
		{
			ServiceInstanceId: "datarepository",
			ServiceName:       models.SERVICENAME_NUDM_UEAU,
			Versions: []models.NFServiceVersion{
				{
					ApiFullVersion:  "1",
					ApiVersionInUri: "versionUri",
				},
			},
			Scheme:          "https",
			NfServiceStatus: models.NFSERVICESTATUS_REGISTERED,
			ApiPrefix:       openapi.PtrString(udmUri1),
			IpEndPoints: []models.IpEndPoint{
				{
					Ipv4Address: openapi.PtrString("10.0.13.1"),
					Transport:   models.TRANSPORTPROTOCOL_TCP.Ptr(),
					Port:        openapi.PtrInt32(8090),
				},
			},
		},
	}
	udmProfile1.NfServices = services1
	nfInstances1 := []models.NFProfileDiscovery{
		udmProfile1,
	}
	searchResult1 := models.NewSearchResult(7, nfInstances1)
	udmProfile2 := models.NFProfileDiscovery{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId:  "9999-4343-43-434-343",
		Ipv4Addresses: []string{udmUri2},
		NfType:        "UDM",
		NfStatus:      "REGISTERED",
	}
	services2 := []models.NFService{
		{
			ServiceInstanceId: "datarepository",
			ServiceName:       models.SERVICENAME_NUDM_UEAU,
			Versions: []models.NFServiceVersion{
				{
					ApiFullVersion:  "1",
					ApiVersionInUri: "versionUri",
				},
			},
			Scheme:          "https",
			NfServiceStatus: models.NFSERVICESTATUS_REGISTERED,
			ApiPrefix:       openapi.PtrString(udmUri2),
			IpEndPoints: []models.IpEndPoint{
				{
					Ipv4Address: openapi.PtrString("20.20.13.1"),
					Transport:   models.TRANSPORTPROTOCOL_TCP.Ptr(),
					Port:        openapi.PtrInt32(8090),
				},
			},
		},
	}
	udmProfile2.NfServices = services2
	nfInstances2 := []models.NFProfileDiscovery{
		udmProfile2,
	}
	searchResult2 := models.NewSearchResult(7, nfInstances2)
	defer func() {
		consumer.NRFCacheSearchNFInstances = origNRFCacheSearchNFInstances
		consumer.SendNfDiscoveryToNrf = origSendNfDiscoveryToNrf
	}()
	consumer.NRFCacheSearchNFInstances = func(ctx context.Context, nrfUri string, targetNfType, requestNfType models.NFType, param Nnrf_NFDiscovery.ApiSearchNFInstancesRequest) (*models.SearchResult, error) {
		t.Logf("test SearchNFInstance called")
		callCountSearchNFInstances++
		return searchResult1, nil
	}
	consumer.SendNfDiscoveryToNrf = func(ctx context.Context, nrfUri string, targetNfType, requestNfType models.NFType, configure consumer.SearchNFInstancesRequestConfigurer) (*models.SearchResult, error) {
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
			if callCountSearchNFInstances != parameters[i].expectedCallCountSearchNFInstances {
				t.Errorf("NF instance search count mismatch. got = %d, want = %d (NF instance is searched in the cache)",
					callCountSearchNFInstances, parameters[i].expectedCallCountSearchNFInstances)
			}
			if callCountSendNfDiscovery != parameters[i].expectedCallCountSendNfDiscovery {
				t.Errorf("NF discovery request count mismatch. got = %d, want = %d (NF discovery request is sent to NRF)",
					callCountSendNfDiscovery, parameters[i].expectedCallCountSendNfDiscovery)
			}
			if udm_uri != parameters[i].udmUri {
				t.Errorf("UDM URI mismatch. got = %q, want = %q (UDR Uri is set)",
					udm_uri, parameters[i].udmUri)
			}
			callCountSendNfDiscovery = 0
			callCountSearchNFInstances = 0
		})
	}
}

func TestGetUDMUri_SkipsInstancesWithoutUsableEndpoints(t *testing.T) {
	origSendSearchNFInstances := consumer.SendSearchNFInstances
	origSendNfDiscoveryToNrf := consumer.SendNfDiscoveryToNrf
	defer func() {
		consumer.SendSearchNFInstances = origSendSearchNFInstances
		consumer.SendNfDiscoveryToNrf = origSendNfDiscoveryToNrf
	}()

	consumer.SendSearchNFInstances = func(nrfUri string, targetNfType, requestNfType models.NFType,
		configure consumer.SearchNFInstancesRequestConfigurer,
	) (*models.SearchResult, error) {
		invalidProfile := models.NFProfileDiscovery{
			NfInstanceId: "empty-service-instance",
			NfType:       models.NFTYPE_UDM,
			NfStatus:     models.NFSTATUS_REGISTERED,
			NfServices: []models.NFService{{
				ServiceName: models.SERVICENAME_NUDM_UEAU,
				Scheme:      models.URISCHEME_HTTPS,
			}},
		}
		validProfile := models.NFProfileDiscovery{
			NfInstanceId: "usable-instance",
			NfType:       models.NFTYPE_UDM,
			NfStatus:     models.NFSTATUS_REGISTERED,
			NfServices: []models.NFService{{
				ServiceName: models.SERVICENAME_NUDM_UEAU,
				Scheme:      models.URISCHEME_HTTPS,
				IpEndPoints: []models.IpEndPoint{{
					Ipv4Address: openapi.PtrString("20.20.13.1"),
					Port:        openapi.PtrInt32(8090),
				}},
			}},
		}
		return models.NewSearchResult(2, []models.NFProfileDiscovery{invalidProfile, validProfile}), nil
	}
	consumer.SendNfDiscoveryToNrf = func(ctx context.Context, nrfUri string, targetNfType, requestNfType models.NFType,
		configure consumer.SearchNFInstancesRequestConfigurer,
	) (*models.SearchResult, error) {
		t.Fatal("did not expect direct NRF discovery fallback")
		return nil, nil
	}

	if got := producer.GetUdmUrl(ausfContext.GetSelf().NrfUri); got != "https://20.20.13.1:8090" {
		t.Fatalf("unexpected UDM URL: got %q want %q", got, "https://20.20.13.1:8090")
	}
}

func TestCreateSubscriptionSuccess(t *testing.T) {
	t.Logf("test cases for CreateSubscription")
	udmProfile := models.NFProfileDiscovery{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId: nfInstanceID,
		NfType:       "UDM",
		NfStatus:     "REGISTERED",
	}
	nfInstances := []models.NFProfileDiscovery{
		udmProfile,
	}
	searchResult := models.NewSearchResult(7, nfInstances)
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
	consumer.StoreApiSearchNFInstances = func(*Nnrf_NFDiscovery.NFInstancesStoreAPIService, Nnrf_NFDiscovery.ApiSearchNFInstancesRequest) (*models.SearchResult, *http.Response, error) {
		t.Logf("test SearchNFInstances called")
		return searchResult, &httpResponse, nil
	}
	consumer.CreateSubscription = func(nrfUri string, nrfSubscriptionData models.SubscriptionData) (nrfSubData *models.SubscriptionData, problemDetails *models.ProblemDetails, err error) {
		t.Logf("test SendCreateSubscription called")
		callCountSendCreateSubscription++
		subscriptionData := models.NewSubscriptionData("https://:0/nausf-callback/v1/nf-status-notify")
		subscriptionData.SetReqNfType("AUSF")
		subscriptionData.SetSubscriptionId(subscriptionID)
		return subscriptionData, nil, nil
	}
	// NRF caching is disabled
	ausfContext.GetSelf().EnableNrfCaching = false
	configureSearchUDMRequest := func(request Nnrf_NFDiscovery.ApiSearchNFInstancesRequest) Nnrf_NFDiscovery.ApiSearchNFInstancesRequest {
		return request.ServiceNames([]models.ServiceName{models.SERVICENAME_NUDM_UEAU})
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
			_, err := consumer.SendNfDiscoveryToNrf(context.Background(), "testNRFUri", "UDM", "AUSF", configureSearchUDMRequest)
			val, _ := ausfContext.GetSelf().NfStatusSubscriptions.Load(parameters[i].nfInstanceId)
			if val != parameters[i].subscriptionId {
				t.Errorf("Subscription ID mismatch. got = %v, want = %v (Correct Subscription ID is not stored in the AUSF context)",
					val, parameters[i].subscriptionId)
			}
			if err != parameters[i].expectedError {
				t.Errorf("SendNfDiscoveryToNrf error mismatch. got = %v, want = %v (SendNfDiscoveryToNrf is failed)",
					err, parameters[i].expectedError)
			}
			if callCountSendCreateSubscription != parameters[i].expectedCallCountSendCreateSubscription {
				t.Errorf("Subscription creation count mismatch. got = %d, want = %d (Subscription is not created for NF instance)",
					callCountSendCreateSubscription, parameters[i].expectedCallCountSendCreateSubscription)
			}
			callCountSendCreateSubscription = 0
		})
	}
}

func TestSendNfDiscoveryToNrf_WhenSearchResultIsNil_ReturnsError(t *testing.T) {
	origStoreApiSearchNFInstances := consumer.StoreApiSearchNFInstances
	defer func() {
		consumer.StoreApiSearchNFInstances = origStoreApiSearchNFInstances
	}()

	consumer.StoreApiSearchNFInstances = func(*Nnrf_NFDiscovery.NFInstancesStoreAPIService, Nnrf_NFDiscovery.ApiSearchNFInstancesRequest) (*models.SearchResult, *http.Response, error) {
		return nil, &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("{}")),
		}, nil
	}

	result, err := consumer.SendNfDiscoveryToNrf(context.Background(), "testNRFUri", models.NFTYPE_UDM, models.NFTYPE_AUSF, nil)
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
	if err == nil || !strings.Contains(err.Error(), "nil result") {
		t.Fatalf("expected nil result error, got %v", err)
	}
}

func TestCreateSubscriptionFail(t *testing.T) {
	t.Logf("test cases for CreateSubscription")
	udmProfile := models.NFProfileDiscovery{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId: "84343-4343-43-434-343",
		NfType:       "UDM",
		NfStatus:     "REGISTERED",
	}
	nfInstances := []models.NFProfileDiscovery{
		udmProfile,
	}
	searchResultPtr := models.NewSearchResult(7, nfInstances)
	searchResult := *searchResultPtr
	emptySearchResult := models.SearchResult{}
	nrfSubscriptionDataPtr := models.NewSubscriptionData("https://:0/nausf-callback/v1/nf-status-notify")
	nrfSubscriptionDataPtr.SetReqNfType("AUSF")
	nrfSubscriptionDataPtr.SetSubscriptionId("")
	nrfSubscriptionData := *nrfSubscriptionDataPtr
	emptyNrfSubscriptionData := models.SubscriptionData{}
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
	serverErrorProblemPtr := models.NewProblemDetails()
	serverErrorProblemPtr.SetStatus(http.StatusInternalServerError)
	serverErrorProblemPtr.SetCause("Server Error")
	serverErrorProblemPtr.SetDetail("")
	serverErrorProblem := *serverErrorProblemPtr
	callCountSendCreateSubscription := 0
	origStoreApiSearchNFInstances := consumer.StoreApiSearchNFInstances
	origCreateSubscription := consumer.CreateSubscription
	defer func() {
		consumer.StoreApiSearchNFInstances = origStoreApiSearchNFInstances
		consumer.CreateSubscription = origCreateSubscription
	}()
	// NRF caching is disabled
	ausfContext.GetSelf().EnableNrfCaching = false
	configureSearchUDMRequest := func(request Nnrf_NFDiscovery.ApiSearchNFInstancesRequest) Nnrf_NFDiscovery.ApiSearchNFInstancesRequest {
		return request.ServiceNames([]models.ServiceName{models.SERVICENAME_NUDM_UEAU})
	}
	parameters := []struct {
		httpResponse                            http.Response
		expectedSubscriptionId                  any
		subscriptionError                       error
		expectedError                           error
		subscriptionProblem                     *models.ProblemDetails
		nrfSubscriptionData                     models.SubscriptionData
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
			nil,
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
			nil,
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
			consumer.StoreApiSearchNFInstances = func(*Nnrf_NFDiscovery.NFInstancesStoreAPIService, Nnrf_NFDiscovery.ApiSearchNFInstancesRequest) (*models.SearchResult, *http.Response, error) {
				t.Logf("test SearchNFInstances called")
				return &parameters[i].searchResult, &parameters[i].httpResponse, nil
			}

			consumer.CreateSubscription = func(nrfUri string, nrfSubscriptionData models.SubscriptionData) (nrfSubData *models.SubscriptionData, problemDetails *models.ProblemDetails, err error) {
				t.Logf("test SendCreateSubscription called")
				callCountSendCreateSubscription++
				return &parameters[i].nrfSubscriptionData, parameters[i].subscriptionProblem, parameters[i].subscriptionError
			}
			_, err := consumer.SendNfDiscoveryToNrf(context.Background(), "testNRFUri", "UDM", "AUSF", configureSearchUDMRequest)
			val, _ := ausfContext.GetSelf().NfStatusSubscriptions.Load(udmProfile.NfInstanceId)
			if val != parameters[i].expectedSubscriptionId {
				t.Errorf("Subscription ID mismatch. got = %v, want = %v (Correct Subscription ID is not stored in the AUSF context)",
					val, parameters[i].expectedSubscriptionId)
			}
			if (err != nil || parameters[i].expectedError != nil) &&
				(err == nil || parameters[i].expectedError == nil || err.Error() != parameters[i].expectedError.Error()) {
				t.Errorf("SendNfDiscoveryToNrf error mismatch. got = %v, want = %v (SendNfDiscoveryToNrf is failed)",
					err, parameters[i].expectedError)
			}
			if callCountSendCreateSubscription != parameters[i].expectedCallCountSendCreateSubscription {
				t.Errorf("Subscription creation count mismatch. got = %d, want = %d (Subscription is not created for NF instance)",
					callCountSendCreateSubscription, parameters[i].expectedCallCountSendCreateSubscription)
			}
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
	udmProfile := models.NotificationDataAllOfNfProfile{
		UdmInfo: &models.UdmInfo{
			RoutingIndicators: []string{},
		},
		NfInstanceId: nfInstanceID,
		NfType:       "UDM",
		NfStatus:     "DEREGISTERED",
	}
	badRequestProblemPtr := models.NewProblemDetails()
	badRequestProblemPtr.SetStatus(http.StatusBadRequest)
	badRequestProblemPtr.SetCause("MANDATORY_IE_MISSING")
	badRequestProblemPtr.SetDetail("Missing IE [Event]/[NfInstanceUri] in NotificationData")
	badRequestProblem := *badRequestProblemPtr
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
			if !reflect.DeepEqual(err, parameters[i].expectedProblem) {
				t.Errorf("NfSubscriptionStatusNotifyProcedure error mismatch. got = %+v, want = %+v (NfSubscriptionStatusNotifyProcedure is failed)",
					err, parameters[i].expectedProblem)
			}
			if callCountSendRemoveSubscription != parameters[i].expectedCallCountSendRemoveSubscription {
				t.Errorf("Subscription removal count mismatch. got = %d, want = %d (Subscription is not removed)",
					callCountSendRemoveSubscription, parameters[i].expectedCallCountSendRemoveSubscription)
			}
			if callCountNRFCacheRemoveNfProfileFromNrfCache != parameters[i].expectedCallCountNRFCacheRemoveNfProfileFromNrfCache {
				t.Errorf("NF Profile cache removal count mismatch. got = %d, want = %d (NF Profile is not removed from NRF cache)",
					callCountNRFCacheRemoveNfProfileFromNrfCache, parameters[i].expectedCallCountNRFCacheRemoveNfProfileFromNrfCache)
			}
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
