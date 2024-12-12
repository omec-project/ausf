// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

/*
 * AUSF Configuration Factory
 */

package factory

import (
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/logger"
)

const (
	AUSF_EXPECTED_CONFIG_VERSION = "1.0.0"
)

type Config struct {
	Info          *Info          `yaml:"info"`
	Configuration *Configuration `yaml:"configuration"`
	Logger        *logger.Logger `yaml:"logger"`
	CfgLocation   string
}

type Info struct {
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

const (
	AUSF_DEFAULT_IPV4     = "127.0.0.9"
	AUSF_DEFAULT_PORT     = "8000"
	AUSF_DEFAULT_PORT_INT = 8000
)

type Configuration struct {
	Sbi                      *Sbi            `yaml:"sbi,omitempty"`
	ServiceNameList          []string        `yaml:"serviceNameList,omitempty"`
	NrfUri                   string          `yaml:"nrfUri,omitempty"`
	WebuiUri                 string          `yaml:"webuiUri"`
	GroupId                  string          `yaml:"groupId,omitempty"`
	PlmnSupportList          []models.PlmnId `yaml:"plmnSupportList,omitempty"`
	EnableNrfCaching         bool            `yaml:"enableNrfCaching"`
	NrfCacheEvictionInterval int             `yaml:"nrfCacheEvictionInterval,omitempty"`
	EnableScaling            bool            `yaml:"enableScaling,omitempty"`
}

type Sbi struct {
	Scheme       string `yaml:"scheme"`
	TLS          *TLS   `yaml:"tls"`
	RegisterIPv4 string `yaml:"registerIPv4,omitempty"` // IP that is registered at NRF.
	BindingIPv4  string `yaml:"bindingIPv4,omitempty"`  // IP used to run the server in the node.
	Port         int    `yaml:"port,omitempty"`
}

type TLS struct {
	PEM string `yaml:"pem,omitempty"`
	Key string `yaml:"key,omitempty"`
}

type Security struct {
	IntegrityOrder []string `yaml:"integrityOrder,omitempty"`
	CipheringOrder []string `yaml:"cipheringOrder,omitempty"`
}

func (c *Config) GetVersion() string {
	if c.Info != nil && c.Info.Version != "" {
		return c.Info.Version
	}
	return ""
}
