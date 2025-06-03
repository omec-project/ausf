// SPDX-FileCopyrightText: 2025 Canonical Ltd

// SPDX-License-Identifier: Apache-2.0
//

package polling

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/ausf/nrfregistration"
	"github.com/omec-project/openapi/models"
)

const (
	INITIAL_POLLING_INTERVAL = 5 * time.Second
	POLLING_MAX_BACKOFF      = 40 * time.Second
	POLLING_BACKOFF_FACTOR   = 2
	POLLING_PATH             = "/nfconfig/plmn"
)

// PollNetworkConfig makes a HTTP GET request to the webconsole and updates the network configuration
func PollNetworkConfig() {
	interval := INITIAL_POLLING_INTERVAL
	context := context.GetSelf()

	for {
		time.Sleep(interval)
		newPlmnConfig, err := fetchPlmnConfig()
		if err != nil {
			logger.PollConfigLog.Infoln("error polling network configuration", err)
			interval = minDuration(interval*time.Duration(POLLING_BACKOFF_FACTOR), POLLING_MAX_BACKOFF)
			continue
		}

		interval = INITIAL_POLLING_INTERVAL
		handlePolledPlmnConfig(context, newPlmnConfig)
	}
}

func fetchPlmnConfig() ([]models.PlmnId, error) {
	pollingEndpoint := factory.AusfConfig.Configuration.WebuiUri + POLLING_PATH

	resp, err := http.Get(pollingEndpoint)

	if err != nil {
		return nil, fmt.Errorf("HTTP GET %v failed: %w", pollingEndpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var config []models.PlmnId
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return config, nil
}

func handlePolledPlmnConfig(context *context.AUSFContext, newPlmnConfig []models.PlmnId) {
	if reflect.DeepEqual(context.PlmnList, newPlmnConfig) {
		logger.PollConfigLog.Debugln("PLMN config did not change")
		return
	}
	context.PlmnList = newPlmnConfig
	logger.PollConfigLog.Infoln("PLMN config changed %v", context.PlmnList)
	nrfregistration.HandleNewConfig(context.PlmnList)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
