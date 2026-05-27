package config

var currentConfiguration Configuration

func GetConfiguration() *Configuration {
	return &currentConfiguration
}

func setConfiguration(conf Configuration) {
	currentConfiguration = conf
}
