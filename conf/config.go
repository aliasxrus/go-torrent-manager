package conf

import (
	"github.com/beego/beego/v2/core/logs"
	model "go-torrent-manager/models"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
)

var config model.Config

func Get() model.Config {
	if config.Version != "" {
		return config
	}

	config, err := initConfig()
	if err != nil {
		os.Exit(1)
	}

	config.Version = "0.0.3"
	return config
}

func initConfig() (model.Config, error) {
	data, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		logs.Error("Read yaml file error.", err)
		return config, err
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		logs.Error("Unmarshal yaml config error.", err)
		return config, err
	}

	return config, nil
}
