// Copyright (C) 2024-Present Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nfregistration

import (
	"context"

	"github.com/omec-project/ausf/consumer"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/nrfregistration"
)

var service = newService()

func newService() *nrfregistration.Service[[]models.PlmnId] {
	// The consumer functions are looked up on every call so tests can replace them.
	svc, err := nrfregistration.New(nrfregistration.Config[[]models.PlmnId]{
		NfName:  "AUSF",
		Log:     logger.NrfRegistrationLog,
		IsEmpty: func(plmnConfig []models.PlmnId) bool { return len(plmnConfig) == 0 },
		Register: func(_ context.Context, plmnConfig []models.PlmnId) (*models.NFProfile, error) {
			nfProfile, _, err := consumer.SendRegisterNFInstance(plmnConfig)
			return nfProfile, err
		},
		Update: func(ctx context.Context, patch []models.PatchItem) (*models.NFProfile, *models.ProblemDetails, error) {
			return consumer.SendUpdateNFInstance(ctx, patch)
		},
		Deregister: func(context.Context) error { return consumer.SendDeregisterNFInstance() },
	})
	if err != nil {
		panic(err) // the config above is static, so this is a programming error
	}
	return svc
}

// StartNfRegistrationService starts the registration service. If the new config is empty, the NF
// deregisters from the NRF. Else, it registers to the NRF. It cancels the registration in progress
// so that only one registration process runs at the time.
func StartNfRegistrationService(ctx context.Context, plmnConfigChan <-chan []models.PlmnId) {
	service.Start(ctx, plmnConfigChan)
}

// DeregisterNF stops the heartbeat and deregisters the NF from the NRF.
var DeregisterNF = func() {
	service.Deregister(context.Background())
}
