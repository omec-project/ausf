// Copyright (C) 2026 Intel Corporation
// Copyright 2019 free5GC.org
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/util/nfconfig"
)

var AusfConfig Config

// TODO: Support configuration update from REST api
func InitConfigFactory(f string) error {
	AusfConfig = Config{}

	if err := nfconfig.LoadYAML(f, &AusfConfig); err != nil {
		return err
	}
	webuiUri, err := nfconfig.ResolveWebuiUri(AusfConfig.Configuration.WebuiUri, logger.CfgLog)
	if err != nil {
		return err
	}
	AusfConfig.Configuration.WebuiUri = webuiUri
	return nil
}

func CheckConfigVersion() error {
	return nfconfig.CheckVersion(AusfConfig.GetVersion(), AUSF_EXPECTED_CONFIG_VERSION, logger.CfgLog)
}
