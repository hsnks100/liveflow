package config

// Struct to hold the configuration
type Config struct {
	Whep   Whep         `mapstructure:"whep"`
	RTMP   RTMP         `mapstructure:"rtmp"`
	HLS    HLS          `mapstructure:"hls"`
	Docker DockerConfig `mapstructure:"docker"`
}

type RTMP struct {
	Port int `mapstructure:"port"`
}

type Whep struct {
	Port int `mapstructure:"port"`
}

type HLS struct {
	Port  int  `mapstructure:"port"`
	LLHLS bool `mapstructure:"llhls"`
}

type DockerConfig struct {
	Mode bool `mapstructure:"mode"`
}
