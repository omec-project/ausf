// Copyright 2019 free5GC.org
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"fmt"
	"net/http"

	ausfContext "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/v2/models"
	nrfCache "github.com/omec-project/openapi/v2/nrfcache"
)

var (
	CreateSubscription        = SendCreateSubscription
	NRFCacheSearchNFInstances = nrfCache.SearchNFInstances
	StoreApiSearchNFInstances = (*Nnrf_NFDiscovery.NFInstancesStoreAPIService).SearchNFInstancesExecute
)

type SearchNFInstancesRequestConfigurer func(
	Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) Nnrf_NFDiscovery.ApiSearchNFInstancesRequest

func newNFDiscoveryClient(nrfUri string) *Nnrf_NFDiscovery.APIClient {
	configuration := Nnrf_NFDiscovery.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = nrfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	return Nnrf_NFDiscovery.NewAPIClient(configuration)
}

func buildSearchNFInstancesRequest(
	ctx context.Context,
	client *Nnrf_NFDiscovery.APIClient,
	targetNfType, requestNfType models.NFType,
	configure SearchNFInstancesRequestConfigurer,
) Nnrf_NFDiscovery.ApiSearchNFInstancesRequest {
	request := client.NFInstancesStoreAPI.SearchNFInstances(ctx)
	request = request.TargetNfType(targetNfType)
	request = request.RequesterNfType(requestNfType)
	if configure != nil {
		request = configure(request)
	}
	return request
}

func executeSearchNFInstancesRequest(
	nrfUri string,
	requestNfType models.NFType,
	request Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) (result *models.SearchResult, returnErr error) {
	result, res, returnErr := StoreApiSearchNFInstances(request.ApiService.(*Nnrf_NFDiscovery.NFInstancesStoreAPIService), request)
	if res != nil && res.StatusCode == http.StatusTemporaryRedirect {
		returnErr = fmt.Errorf("temporary redirect for non NRF consumer")
	}
	defer func() {
		if res == nil || res.Body == nil {
			return
		}
		if bodyCloseErr := res.Body.Close(); bodyCloseErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("SearchNFInstances' response body cannot close: %+w", bodyCloseErr)
		}
	}()
	if result == nil {
		if returnErr == nil {
			returnErr = openapi.ReportError("SearchNFInstances returned nil result")
		}
		return nil, returnErr
	}

	ausfSelf := ausfContext.GetSelf()

	for _, nfProfile := range result.NfInstances {
		nfInstanceID := nfProfile.GetNfInstanceId()
		if subscriptionID, ok := ausfSelf.NfStatusSubscriptions.Load(nfInstanceID); ok {
			if subscriptionIDStr, ok := subscriptionID.(string); ok && subscriptionIDStr != "" {
				continue
			}
			ausfSelf.NfStatusSubscriptions.Delete(nfInstanceID)
		}
		{
			nrfSubscriptionData := models.SubscriptionData{
				NfStatusNotificationUri: fmt.Sprintf("%s/nausf-callback/v1/nf-status-notify", ausfSelf.GetIPv4Uri()),
				SubscrCond: &models.SubscrCond{
					NfInstanceIdCond: &models.NfInstanceIdCond{
						NfInstanceId: openapi.PtrString(nfInstanceID),
					},
				},
				ReqNfType: &requestNfType,
			}
			nrfSubData, problemDetails, subErr := CreateSubscription(nrfUri, nrfSubscriptionData)
			if problemDetails != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription to NRF, Problem[%+v]", problemDetails)
				ausfSelf.NfStatusSubscriptions.Delete(nfInstanceID)
				if returnErr == nil && subErr != nil {
					returnErr = subErr
				}
			} else if subErr != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription Error[%+v]", subErr)
				ausfSelf.NfStatusSubscriptions.Delete(nfInstanceID)
				if returnErr == nil {
					returnErr = subErr
				}
			} else if nrfSubData != nil {
				ausfSelf.NfStatusSubscriptions.Store(nfInstanceID, nrfSubData.GetSubscriptionId())
			}
		}
	}

	return result, returnErr
}

var SendSearchNFInstances = func(nrfUri string, targetNfType, requestNfType models.NFType,
	configure SearchNFInstancesRequestConfigurer,
) (*models.SearchResult, error) {
	ctx := context.Background()
	if ausfContext.GetSelf().EnableNrfCaching {
		client := newNFDiscoveryClient(nrfUri)
		request := buildSearchNFInstancesRequest(ctx, client, targetNfType, requestNfType, configure)
		return NRFCacheSearchNFInstances(ctx, nrfUri, targetNfType, requestNfType, request)
	}
	return SendNfDiscoveryToNrf(ctx, nrfUri, targetNfType, requestNfType, configure)
}

var SendNfDiscoveryToNrf = func(ctx context.Context, nrfUri string, targetNfType, requestNfType models.NFType,
	configure SearchNFInstancesRequestConfigurer,
) (*models.SearchResult, error) {
	client := newNFDiscoveryClient(nrfUri)
	request := buildSearchNFInstancesRequest(ctx, client, targetNfType, requestNfType, configure)
	return executeSearchNFInstancesRequest(nrfUri, requestNfType, request)
}

func SendNfDiscoveryToNrfCacheQuery(ctx context.Context, nrfUri string, targetNfType, requestNfType models.NFType,
	request Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) (*models.SearchResult, error) {
	request = request.TargetNfType(targetNfType)
	request = request.RequesterNfType(requestNfType)
	return executeSearchNFInstancesRequest(nrfUri, requestNfType, request)
}
