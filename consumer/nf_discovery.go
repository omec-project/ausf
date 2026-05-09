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
	StoreApiClient            = &Nnrf_NFDiscovery.APIClient{}
	StoreApiSearchNFInstances = (*Nnrf_NFDiscovery.NFInstancesStoreAPIService).SearchNFInstancesExecute
)

var SendSearchNFInstances = func(nrfUri string, targetNfType, requestNfType models.NFType,
	param Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) (*models.SearchResult, error) {
	ctx := context.Background()
	if ausfContext.GetSelf().EnableNrfCaching {
		return NRFCacheSearchNFInstances(ctx, nrfUri, targetNfType, requestNfType, param)
	} else {
		return SendNfDiscoveryToNrf(ctx, nrfUri, targetNfType, requestNfType, param)
	}
}

var SendNfDiscoveryToNrf = func(ctx context.Context, nrfUri string, targetNfType, requestNfType models.NFType,
	param Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) (*models.SearchResult, error) {
	configuration := Nnrf_NFDiscovery.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = nrfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nnrf_NFDiscovery.NewAPIClient(configuration)

	param = param.TargetNfType(targetNfType)
	param = param.RequesterNfType(requestNfType)
	result, res, err := StoreApiSearchNFInstances(client.NFInstancesStoreAPI.(*Nnrf_NFDiscovery.NFInstancesStoreAPIService), param)
	returnErr := err
	if res != nil && res.StatusCode == http.StatusTemporaryRedirect {
		returnErr = fmt.Errorf("temporary redirect for non NRF consumer")
	}
	if res != nil {
		defer func() {
			if bodyCloseErr := res.Body.Close(); bodyCloseErr != nil {
				returnErr = fmt.Errorf("SearchNFInstances' response body cannot close: %+w", bodyCloseErr)
			}
		}()
	}
	if result == nil {
		if returnErr == nil {
			returnErr = openapi.ReportError("SearchNFInstances returned nil result")
		}
		return nil, returnErr
	}

	ausfSelf := ausfContext.GetSelf()

	for _, nfProfile := range result.NfInstances {
		// checking whether the AUSF subscribed to this target nfinstanceid or not
		if _, ok := ausfSelf.NfStatusSubscriptions.Load(nfProfile.NfInstanceId); !ok {
			nrfSubscriptionData := models.SubscriptionData{
				NfStatusNotificationUri: fmt.Sprintf("%s/nausf-callback/v1/nf-status-notify", ausfSelf.GetIPv4Uri()),
				SubscrCond: &models.SubscrCond{
					NfInstanceIdCond: &models.NfInstanceIdCond{
						NfInstanceId: openapi.PtrString(nfProfile.GetNfInstanceId()),
					},
				},
				ReqNfType: &requestNfType,
			}
			nrfSubData, problemDetails, subErr := CreateSubscription(nrfUri, nrfSubscriptionData)
			if problemDetails != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription to NRF, Problem[%+v]", problemDetails)
				ausfSelf.NfStatusSubscriptions.Store(nfProfile.GetNfInstanceId(), "")
				if returnErr == nil && subErr != nil {
					returnErr = subErr
				}
			} else if subErr != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription Error[%+v]", subErr)
				ausfSelf.NfStatusSubscriptions.Store(nfProfile.GetNfInstanceId(), "")
				if returnErr == nil {
					returnErr = subErr
				}
			} else if nrfSubData != nil {
				ausfSelf.NfStatusSubscriptions.Store(nfProfile.GetNfInstanceId(), nrfSubData.GetSubscriptionId())
			}
		}
	}

	return result, returnErr
}
