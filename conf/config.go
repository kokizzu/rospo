package conf

import (
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

// Config holds all the config values
type Config struct {
	SshClient *SshClientConf `yaml:"sshclient"`
	Tunnel    []*TunnnelConf `yaml:"tunnel"`
	SshD      *SshDConf      `yaml:"sshd"`
}

func LoadConfig(filePath string) *Config {
	f, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Error while reading config file: %s", err)
	}
	defer f.Close()

	// set some reasonable defaults
	cfg := Config{
		&SshClientConf{
			Insecure: false,
		},
		nil,
		nil,
	}

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&cfg)
	if err != nil {
		log.Fatalf("Error while parsing config file: %s", err)
	}
	return &cfg
}
