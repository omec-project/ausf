// Copyright 2019 free5GC.org
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	ausfContext "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/Nnrf_NFManagement"
	"github.com/omec-project/openapi/models"
)

func buildNFInstance(ausfContext *ausfContext.AUSFContext, plmnConfig []models.PlmnId) (profile models.NfProfile, err error) {
	profile.NfInstanceId = ausfContext.NfId
	profile.NfType = models.NfType_AUSF
	profile.NfStatus = models.NfStatus_REGISTERED
	profile.Ipv4Addresses = append(profile.Ipv4Addresses, ausfContext.RegisterIPv4)
	services := []models.NfService{}
	for _, nfService := range ausfContext.NfService {
		services = append(services, nfService)
	}
	if len(services) > 0 {
		profile.NfServices = &services
	}
	var ausfInfo models.AusfInfo
	ausfInfo.GroupId = ausfContext.GroupID
	profile.AusfInfo = &ausfInfo
	profile.PlmnList = &plmnConfig
	return
}

var SendRegisterNFInstance = func(plmnConfig []models.PlmnId) (prof models.NfProfile, resourceNrfUri string, err error) {
	self := ausfContext.GetSelf()
	profile, err := buildNFInstance(self, plmnConfig)
	if err != nil {
		return profile, "", err
	}

	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(self.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	nfProfile, res, err := client.NFInstanceIDDocumentApi.RegisterNFInstance(context.TODO(), profile.NfInstanceId, profile)
	logger.ConsumerLog.Debugln("RegisterNFInstance done using profile:", profile)
	if err != nil {
		return nfProfile, "", err
	}

	if res == nil {
		return nfProfile, "", openapi.ReportError("no response from server")
	}

	defer func() {
		if resCloseErr := res.Body.Close(); resCloseErr != nil {
			logger.ConsumerLog.Errorf("AUSF NFInstanceIDDocumentApi response body cannot close: %+v", resCloseErr)
		}
	}()

	switch res.StatusCode {
	case http.StatusOK: // NFUpdate
		logger.ConsumerLog.Debugln("AUSF NF profile updated with complete replacement")
		return nfProfile, "", nil
	case http.StatusCreated: // NFRegister
		resourceUri := res.Header.Get("Location")
		resourceNrfUri = resourceUri[:strings.Index(resourceUri, "/nnrf-nfm/")]
		retrieveNfInstanceId := resourceUri[strings.LastIndex(resourceUri, "/")+1:]
		self.NfId = retrieveNfInstanceId
		logger.ConsumerLog.Debugln("AUSF NF profile registered to the NRF")
		return nfProfile, resourceNrfUri, nil
	default:
		return nfProfile, "", openapi.ReportError("unexpected status code returned by the NRF %d", res.StatusCode)
	}
}

var SendDeregisterNFInstance = func() (*models.ProblemDetails, error) {
	logger.AppLog.Infoln("send Deregister NFInstance")

	ausfSelf := ausfContext.GetSelf()
	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(ausfSelf.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	res, err := client.NFInstanceIDDocumentApi.DeregisterNFInstance(context.Background(), ausfSelf.NfId)
	if err == nil {
		if res != nil && res.StatusCode == 204 {
			defer res.Body.Close()
			return nil, nil
		}
		return nil, openapi.ReportError("unexpected response code")
	}
	if res == nil {
		return nil, openapi.ReportError("no response from server")
	}

	defer func() {
		if resCloseErr := res.Body.Close(); resCloseErr != nil {
			logger.ConsumerLog.Errorf("NFInstanceIDDocumentApi response body cannot close: %+v", resCloseErr)
		}
	}()

	if openapiErr, ok := err.(openapi.GenericOpenAPIError); ok {
		if model := openapiErr.Model(); model != nil {
			if problem, ok := model.(models.ProblemDetails); ok {
				return &problem, nil
			}
		}
	}

	return nil, err
}

var SendUpdateNFInstance = func(patchItem []models.PatchItem) (nfProfile models.NfProfile, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugln("send Update NFInstance")

	ausfSelf := ausfContext.GetSelf()
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(ausfSelf.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	var res *http.Response
	nfProfile, res, err = client.NFInstanceIDDocumentApi.UpdateNFInstance(context.Background(), ausfSelf.NfId, patchItem)
	if err == nil {
		if res != nil && (res.StatusCode == 200 || res.StatusCode == 204) {
			defer res.Body.Close()
			return nfProfile, nil, nil
		}
		return nfProfile, nil, openapi.ReportError("unexpected response code")
	}
	if res == nil {
		return nfProfile, nil, openapi.ReportError("no response from server")
	}

	defer func() {
		if resCloseErr := res.Body.Close(); resCloseErr != nil {
			logger.ConsumerLog.Errorf("UpdateNFInstance response cannot close: %+v", resCloseErr)
		}
	}()
	if openapiErr, ok := err.(openapi.GenericOpenAPIError); ok {
		if model := openapiErr.Model(); model != nil {
			if problem, ok := model.(models.ProblemDetails); ok {
				return nfProfile, &problem, nil
			}
		}
	}
	return nfProfile, nil, err
}

var SendCreateSubscription = func(nrfUri string, nrfSubscriptionData models.NrfSubscriptionData) (nrfSubData models.NrfSubscriptionData, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugln("send Create Subscription")

	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(nrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	var res *http.Response
	nrfSubData, res, err = client.SubscriptionsCollectionApi.CreateSubscription(context.TODO(), nrfSubscriptionData)
	if err == nil {
		return
	} else if res != nil {
		defer func() {
			if resCloseErr := res.Body.Close(); resCloseErr != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription response cannot close: %+v", resCloseErr)
			}
		}()
		if res.Status != err.Error() {
			logger.ConsumerLog.Errorf("SendCreateSubscription received error response: %v", res.Status)
			return
		}
		problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}
	return
}

var SendRemoveSubscription = func(subscriptionId string) (problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Infoln("send Remove Subscription")

	ausfSelf := ausfContext.GetSelf()
	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(ausfSelf.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)
	var res *http.Response

	res, err = client.SubscriptionIDDocumentApi.RemoveSubscription(context.Background(), subscriptionId)
	if err == nil {
		return
	} else if res != nil {
		defer func() {
			if bodyCloseErr := res.Body.Close(); bodyCloseErr != nil {
				err = fmt.Errorf("RemoveSubscription's response body cannot close: %w", bodyCloseErr)
			}
		}()
		if res.Status != err.Error() {
			return
		}
		problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}
	return
}
