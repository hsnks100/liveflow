package config

// Struct to hold the configuration
type Config struct {
	RTMP      RTMP         `mapstructure:"rtmp"`
	Service   Service      `mapstructure:"service"`
	Docker    DockerConfig `mapstructure:"docker"`
	MP4       MP4          `mapstructure:"mp4"`
	EBML      EBML         `mapstructure:"ebml"`
	Thumbnail Thumbnail    `mapstructure:"thumbnail"`
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

// Thumbnail configuration for thumbnail generation service
type Thumbnail struct {
	Enable          bool   `mapstructure:"enable"`
	OutputPath      string `mapstructure:"output_path"`
	IntervalSeconds int    `mapstructure:"interval_seconds"`
	Width           int    `mapstructure:"width"`
	Height          int    `mapstructure:"height"`
}
