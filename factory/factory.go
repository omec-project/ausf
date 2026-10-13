// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

/*
 * AUSF Configuration Factory
 */

package factory

import (
	"net/url"
	"os"

	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi/v2"
	"go.yaml.in/yaml/v4"
)

var AusfConfig Config

const defaultWebuiUri = "http://webui:5001"

// TODO: Support configuration update from REST api
func InitConfigFactory(f string) error {
	content, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	AusfConfig = Config{}

	if err = yaml.Unmarshal(content, &AusfConfig); err != nil {
		return err
	}
	if AusfConfig.Configuration.WebuiUri == "" {
		AusfConfig.Configuration.WebuiUri = defaultWebuiUri
		logger.CfgLog.Infof("webuiUri not set in configuration file. Using %v", AusfConfig.Configuration.WebuiUri)
		return nil
	}
	err = validateWebuiUri(AusfConfig.Configuration.WebuiUri)
	return err
}

func CheckConfigVersion() error {
	currentVersion := AusfConfig.GetVersion()

	if currentVersion != AUSF_EXPECTED_CONFIG_VERSION {
		return openapi.ReportError("config version is [%s], but expected is [%s]",
			currentVersion, AUSF_EXPECTED_CONFIG_VERSION)
	}
	logger.CfgLog.Infof("config version [%s]", currentVersion)

	return nil
}

func validateWebuiUri(uri string) error {
	parsedUrl, err := url.ParseRequestURI(uri)
	if err != nil {
		return err
	}
	if parsedUrl.Scheme != "http" && parsedUrl.Scheme != "https" {
		return openapi.ReportError("unsupported scheme for webuiUri: %s", parsedUrl.Scheme)
	}
	if parsedUrl.Hostname() == "" {
		return openapi.ReportError("missing host in webuiUri")
	}
	return nil
}
