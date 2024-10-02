// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

/*
 *
 * AUSF Service
 *
 * API version: 1.0.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
	"go.uber.org/zap"

	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/ausf/service"
)

var AUSF = &service.AUSF{}

var appLog *zap.SugaredLogger

func init() {
	appLog = logger.AppLog
}

func main() {
	app := cli.NewApp()
	app.Name = "ausf"
	appLog.Infoln(app.Name)
	app.Usage = "-free5gccfg common configuration file -ausfcfg ausf configuration file"
	app.Action = action
	app.Flags = AUSF.GetCliCmd()
	if err := app.Run(os.Args); err != nil {
		appLog.Errorf("AUSF Run error: %v", err)
	}
}

func action(c *cli.Context) error {
	if err := AUSF.Initialize(c); err != nil {
		logger.CfgLog.Errorf("%+v", err)
		return fmt.Errorf("failed to initialize")
	}

	AUSF.Start()

	return nil
}
