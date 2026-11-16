// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd.
// Copyright 2019 free5GC.org
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"

	ausfContext "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/nrfclient"
)

func newNrfClient(nrfUri string) *nrfclient.Client {
	return nrfclient.New(nrfUri, nrfclient.WithLogger(logger.ConsumerLog))
}

func getNfProfile(ausfContext *ausfContext.AUSFContext, plmnConfig []models.PlmnId) (profile models.NFProfile, err error) {
	if ausfContext == nil {
		return profile, openapi.ReportError("ausf context has not been initialized. NF profile cannot be built")
	}
	profile.SetNfInstanceId(ausfContext.NfId)
	profile.SetNfType(models.NFTYPE_AUSF)
	profile.SetNfStatus(models.NFSTATUS_REGISTERED)
	profile.SetIpv4Addresses([]string{ausfContext.RegisterIPv4})
	services := map[string]models.NFService{}
	serviceList := []models.NFService{}
	for _, nfService := range ausfContext.NfService {
		services[nfService.GetServiceInstanceId()] = nfService
		serviceList = append(serviceList, nfService)
	}
	if len(services) > 0 {
		profile.SetNfServices(serviceList)
		profile.SetNfServiceList(services)
	}
	ausfInfo := models.NewAusfInfo()
	ausfInfo.SetGroupId(ausfContext.GroupID)
	profile.SetAusfInfo(*ausfInfo)
	profile.SetPlmnList(plmnConfig)
	return profile, err
}

var SendRegisterNFInstance = func(plmnConfig []models.PlmnId) (prof *models.NFProfile, resourceNrfUri string, err error) {
	self := ausfContext.GetSelf()
	nfProfile, err := getNfProfile(self, plmnConfig)
	if err != nil {
		return models.NewNFProfileWithDefaults(), "", err
	}

	result, err := newNrfClient(self.NrfUri).Register(context.TODO(), nfProfile)
	if err != nil {
		return models.NewNFProfileWithDefaults(), "", err
	}
	if result.Created {
		logger.ConsumerLog.Debugln("AUSF NF profile registered to the NRF")
		if result.NfInstanceID != "" {
			self.NfId = result.NfInstanceID
		}
	} else {
		logger.ConsumerLog.Debugln("AUSF NF profile updated with complete replacement")
	}
	return result.Profile, result.NrfURI, nil
}

var SendDeregisterNFInstance = func() error {
	logger.ConsumerLog.Infoln("send Deregister NFInstance")

	ausfSelf := ausfContext.GetSelf()
	return newNrfClient(ausfSelf.NrfUri).Deregister(context.Background(), ausfSelf.NfId)
}

var SendUpdateNFInstance = func(ctx context.Context, patchItem []models.PatchItem) (receivedNfProfile *models.NFProfile, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugln("send Update NFInstance")

	ausfSelf := ausfContext.GetSelf()
	return newNrfClient(ausfSelf.NrfUri).Update(ctx, ausfSelf.NfId, patchItem)
}

var SendCreateSubscription = func(nrfUri string, nrfSubscriptionData models.SubscriptionData) (nrfSubData *models.SubscriptionData, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugln("send Create Subscription")

	return newNrfClient(nrfUri).CreateSubscription(context.TODO(), nrfSubscriptionData)
}

var SendRemoveSubscription = func(subscriptionId string) (problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Infoln("send Remove Subscription")

	ausfSelf := ausfContext.GetSelf()
	return newNrfClient(ausfSelf.NrfUri).RemoveSubscription(context.Background(), subscriptionId)
}
