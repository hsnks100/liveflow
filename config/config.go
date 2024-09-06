package config

// Struct to hold the configuration
type Config struct {
	Whep   ServerConfig `mapstructure:"whep"`
	RTMP   ServerConfig `mapstructure:"rtmp"`
	HLS    ServerConfig `mapstructure:"hls"`
	Docker DockerConfig `mapstructure:"docker"`
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type DockerConfig struct {
	Mode bool `mapstructure:"mode"`
}
