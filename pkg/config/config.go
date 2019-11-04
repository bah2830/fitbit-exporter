package config

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Fitbit struct {
		ClientID      string `yaml:"clientId"`
		ClientSecret  string `yaml:"clientSecret"`
		BackfillStart string `yaml:"backfillStart"`
	} `yaml:"fitbit"`
	Database struct {
		Host     string
		Username string
		Password string
		Database string
	}
}

func LoadConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}
