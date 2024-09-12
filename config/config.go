package config

// Struct to hold the configuration
type Config struct {
	RTMP    RTMP         `mapstructure:"rtmp"`
	Service Service      `mapstructure:"service"`
	Docker  DockerConfig `mapstructure:"docker"`
	MP4     MP4          `mapstructure:"mp4"`
	EBML    EBML         `mapstructure:"ebml"`
}

type RTMP struct {
	Port int `mapstructure:"port"`
}

type Service struct {
	Port    int  `mapstructure:"port"`
	LLHLS   bool `mapstructure:"llhls"`
	DiskRam bool `mapstructure:"disk_ram"`
}

type DockerConfig struct {
	Mode bool `mapstructure:"mode"`
}

type MP4 struct {
	Record bool `mapstructure:"record"`
}

type EBML struct {
	Record bool `mapstructure:"record"`
}
