// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/omec-project/ausf/factory"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/ausf/util"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/path_util"
)

func TestInit() {
	// load config
	DefaultAusfConfigPath := path_util.Free5gcPath("free5gc/config/ausfcfg.yaml")
	if err := factory.InitConfigFactory(DefaultAusfConfigPath); err != nil {
		panic(err)
	}
	Init()
}

func InitAusfContext(context *AUSFContext) {
	config := factory.AusfConfig
	logger.InitLog.Infof("ausfconfig Info: Version[%s] Description[%s]", config.Info.Version, config.Info.Description)

	configuration := config.Configuration
	sbi := configuration.Sbi

	context.NfId = uuid.New().String()
	context.GroupID = configuration.GroupId
	context.NrfUri = configuration.NrfUri
	context.UriScheme = models.UriScheme(configuration.Sbi.Scheme) // default uri scheme
	context.RegisterIPv4 = factory.AUSF_DEFAULT_IPV4               // default localhost
	context.SBIPort = factory.AUSF_DEFAULT_PORT_INT                // default port
	context.Key = util.AusfKeyPath                                 // default key path
	context.PEM = util.AusfPemPath                                 // default PEM path
	if sbi != nil {
		if sbi.RegisterIPv4 != "" {
			context.RegisterIPv4 = sbi.RegisterIPv4
		}
		if sbi.Port != 0 {
			context.SBIPort = sbi.Port
		}

		if sbi.Scheme == "https" {
			context.UriScheme = models.UriScheme_HTTPS
		} else {
			context.UriScheme = models.UriScheme_HTTP
		}
		if tls := sbi.TLS; tls != nil {
			if tls.Key != "" {
				context.Key = tls.Key
			}
			if tls.PEM != "" {
				context.PEM = tls.PEM
			}
		}

		context.BindingIPv4 = os.Getenv(sbi.BindingIPv4)
		if context.BindingIPv4 != "" {
			logger.InitLog.Infoln("parsing ServerIPv4 address from ENV Variable")
		} else {
			context.BindingIPv4 = sbi.BindingIPv4
			if context.BindingIPv4 == "" {
				logger.InitLog.Warnln("error parsing ServerIPv4 address as string. Using the 0.0.0.0 address as default")
				context.BindingIPv4 = "0.0.0.0"
			}
		}
	}

	context.Url = string(context.UriScheme) + "://" + context.RegisterIPv4 + ":" + strconv.Itoa(context.SBIPort)
	roc := os.Getenv("MANAGED_BY_CONFIG_POD")
	if roc != "true" {
		context.PlmnList = append(context.PlmnList, configuration.PlmnSupportList...)
	}
	context.EnableScaling = configuration.EnableScaling
	context.EnableNrfCaching = configuration.EnableNrfCaching
	if configuration.EnableNrfCaching {
		if configuration.NrfCacheEvictionInterval == 0 {
			context.NrfCacheEvictionInterval = time.Duration(900) // 15 mins
		} else {
			context.NrfCacheEvictionInterval = time.Duration(configuration.NrfCacheEvictionInterval)
		}
	}

	// context.NfService
	context.NfService = make(map[models.ServiceName]models.NfService)
	AddNfServices(&context.NfService, &config, context)
	logger.ContextLog.Infoln("ausf context:", context)
}

func AddNfServices(serviceMap *map[models.ServiceName]models.NfService, config *factory.Config, context *AUSFContext) {
	var nfService models.NfService
	var ipEndPoints []models.IpEndPoint
	var nfServiceVersions []models.NfServiceVersion
	services := *serviceMap

	// nausf-auth
	nfService.ServiceInstanceId = context.NfId
	nfService.ServiceName = models.ServiceName_NAUSF_AUTH

	var ipEndPoint models.IpEndPoint
	ipEndPoint.Ipv4Address = context.RegisterIPv4
	ipEndPoint.Port = int32(context.SBIPort)
	ipEndPoints = append(ipEndPoints, ipEndPoint)

	var nfServiceVersion models.NfServiceVersion
	nfServiceVersion.ApiFullVersion = config.Info.Version
	nfServiceVersion.ApiVersionInUri = "v1"
	nfServiceVersions = append(nfServiceVersions, nfServiceVersion)

	nfService.Scheme = context.UriScheme
	nfService.NfServiceStatus = models.NfServiceStatus_REGISTERED

	nfService.IpEndPoints = &ipEndPoints
	nfService.Versions = &nfServiceVersions
	services[models.ServiceName_NAUSF_AUTH] = nfService
}
