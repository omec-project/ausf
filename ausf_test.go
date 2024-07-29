// SPDX-FileCopyrightText: 2024 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//
/*
 * AUSF Unit Testcases
 *
 */
package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/omec-project/ausf/consumer"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/service"
	"github.com/omec-project/openapi/models"
	"github.com/stretchr/testify/require"
)

var AUSFTest = &service.AUSF{}

func init() {
	if err := os.Setenv("POD_IP", "127.0.0.1"); err != nil {
		fmt.Printf("Could not set env POD_IP: %+v\n", err)
	}
	if err := factory.InitConfigFactory("factory/ausfcfg.yaml"); err != nil {
		fmt.Printf("Could not InitConfigFactory: %+v\n", err)
	}
}

func TestRegisterNF(t *testing.T) {
	// Save current function and restore at the end:
	origRegisterNFInstance := consumer.SendRegisterNFInstance
	// origSearchNFInstances := consumer.SendSearchNFInstances
	origUpdateNFInstance := consumer.SendUpdateNFInstance
	defer func() {
		consumer.SendRegisterNFInstance = origRegisterNFInstance
		// consumer.SendSearchNFInstances = origSearchNFInstances
		consumer.SendUpdateNFInstance = origUpdateNFInstance
	}()
	fmt.Printf("test case TestRegisterNF \n")
	var prof models.NfProfile
	consumer.SendRegisterNFInstance = func(nrfUri string, nfInstanceId string, profile models.NfProfile) (models.NfProfile, string, string, error) {
		prof = profile
		prof.HeartBeatTimer = 1
		fmt.Printf("Test RegisterNFInstance called\n")
		return prof, "", "", nil
	}
	/*consumer.SendSearchNFInstances = func(nrfUri string, targetNfType, requestNfType models.NfType, param Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (*models.SearchResult, error) {
		fmt.Printf("Test SearchNFInstance called\n")
		return &models.SearchResult{}, nil
	}*/
	consumer.SendUpdateNFInstance = func(patchItem []models.PatchItem) (nfProfile models.NfProfile, problemDetails *models.ProblemDetails, err error) {
		return prof, nil, nil
	}
	go AUSFTest.RegisterNF()
	service.ConfigPodTrigger <- true
	time.Sleep(5 * time.Second)
	require.Equal(t, service.KeepAliveTimer != nil, true)

	/*service.ConfigPodTrigger <- false
	time.Sleep(1 * time.Second)
	require.Equal(t, service.KeepAliveTimer == nil, true)
	*/
}
