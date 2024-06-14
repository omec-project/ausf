// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"fmt"
	"net/http"

	ausf_context "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/logger"
	nrf_cache "github.com/omec-project/nrf/nrfcache"
	"github.com/omec-project/openapi/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/models"
)

func SendSearchNFInstances(nrfUri string, targetNfType, requestNfType models.NfType,
	param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (models.SearchResult, error) {
	if ausf_context.GetSelf().EnableNrfCaching {
		return nrf_cache.SearchNFInstances(nrfUri, targetNfType, requestNfType, param)
	} else {
		return SendNfDiscoveryToNrf(nrfUri, targetNfType, requestNfType, param)
	}
}

func SendNfDiscoveryToNrf(nrfUri string, targetNfType, requestNfType models.NfType,
	param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (models.SearchResult, error) {
	configuration := Nnrf_NFDiscovery.NewConfiguration()
	configuration.SetBasePath(nrfUri)
	client := Nnrf_NFDiscovery.NewAPIClient(configuration)

	result, rsp, rspErr := client.NFInstancesStoreApi.SearchNFInstances(context.TODO(),
		targetNfType, requestNfType, param)
	if rspErr != nil {
		return result, fmt.Errorf("NFInstancesStoreApi Response error: %+w", rspErr)
	}
	defer func() {
		if rspCloseErr := rsp.Body.Close(); rspCloseErr != nil {
			logger.ConsumerLog.Errorf("NFInstancesStoreApi Response cannot close: %v", rspCloseErr)
		}
	}()
	if rsp != nil && rsp.StatusCode == http.StatusTemporaryRedirect {
		return result, fmt.Errorf("Temporary Redirect For Non NRF Consumer")
	}
	return result, rspErr
}
