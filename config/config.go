package config

// Struct to hold the configuration
type Config struct {
	Monitor Monitor      `mapstructure:"monitor"`
	Whep    Whep         `mapstructure:"whep"`
	RTMP    RTMP         `mapstructure:"rtmp"`
	HLS     HLS          `mapstructure:"hls"`
	Docker  DockerConfig `mapstructure:"docker"`
}

type Monitor struct {
	Port int `mapstructure:"port"`
}

type RTMP struct {
	Port int `mapstructure:"port"`
}

type Whep struct {
	Port int `mapstructure:"port"`
}

type HLS struct {
	Port    int  `mapstructure:"port"`
	LLHLS   bool `mapstructure:"llhls"`
	DiskRam bool `mapstructure:"disk_ram"`
}

type DockerConfig struct {
	Mode bool `mapstructure:"mode"`
}
