/*
 * AUSF Configuration Factory
 */

package factory

import (
	"fmt"
	"io/ioutil"
	"reflect"

	"gopkg.in/yaml.v2"

	"github.com/free5gc/ausf/logger"
)

var AusfConfig Config

// TODO: Support configuration update from REST api
func InitConfigFactory(f string) error {
	if content, err := ioutil.ReadFile(f); err != nil {
		return err
	} else {
		AusfConfig = Config{}

		if yamlErr := yaml.Unmarshal(content, &AusfConfig); yamlErr != nil {
			return yamlErr
		}
	}

	return nil
}

func CheckConfigVersion() error {
	currentVersion := AusfConfig.GetVersion()

	if currentVersion != AUSF_EXPECTED_CONFIG_VERSION {
		return fmt.Errorf("config version is [%s], but expected is [%s].",
			currentVersion, AUSF_EXPECTED_CONFIG_VERSION)
	}
	logger.CfgLog.Infof("config version [%s]", currentVersion)

	return nil
}

func UpdateAusfConfig(f string) error {
	if content, err := ioutil.ReadFile(f); err != nil {
		return err
	} else {
		var ausfConfig Config

		if yamlErr := yaml.Unmarshal(content, &ausfConfig); yamlErr != nil {
			return yamlErr
		}
		if reflect.DeepEqual(AusfConfig.Configuration.Sbi, ausfConfig.Configuration.Sbi) == false {
			logger.CfgLog.Infoln("Updated Sbi is ", ausfConfig.Configuration.Sbi)
		} else if reflect.DeepEqual(AusfConfig.Configuration.ServiceNameList, ausfConfig.Configuration.ServiceNameList) == false {
            logger.CfgLog.Infoln("Updated ServiceName List is ", ausfConfig.Configuration.ServiceNameList)
		} else if reflect.DeepEqual(AusfConfig.Configuration.NrfUri, ausfConfig.Configuration.NrfUri) == false {
            logger.CfgLog.Infoln("Updated NrfUri is ", ausfConfig.Configuration.NrfUri)
		} else if reflect.DeepEqual(AusfConfig.Configuration.PlmnSupportList, ausfConfig.Configuration.PlmnSupportList) == false {
            logger.CfgLog.Infoln("Updated PlmnSupportList is ", ausfConfig.Configuration.PlmnSupportList)
        } else if reflect.DeepEqual(AusfConfig.Configuration.GroupId, ausfConfig.Configuration.GroupId) == false {
            logger.CfgLog.Infoln("Updated GroupId is ", ausfConfig.Configuration.Sbi)
        } 

		AusfConfig = ausfConfig
	}

	return nil
}
