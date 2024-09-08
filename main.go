package main

import (
	"context"
	"fmt"
	"liveflow/config"
	"liveflow/media/streamer/egress/record/mp4"
	"liveflow/media/streamer/egress/record/webm"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"liveflow/httpsrv"
	"liveflow/log"
	"liveflow/media/hlshub"
	"liveflow/media/hub"
	"liveflow/media/streamer/egress/hls"
	"liveflow/media/streamer/egress/whep"
	"liveflow/media/streamer/ingress/rtmp"
	"liveflow/media/streamer/ingress/whip"
)

// RTMP 받으면 자동으로 HLS 서비스 동작, 녹화 서비스까지~?
func main() {
	ctx := context.Background()
	viper.SetConfigName("config") // name of config file (without extension)
	viper.SetConfigType("toml")   // REQUIRED if the config file does not have the extension in the name
	viper.AddConfigPath(".")      // optionally look for config in the working directory
	viper.BindEnv("docker.mode", "DOCKER_MODE")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
	var conf config.Config
	err = viper.Unmarshal(&conf)
	if err != nil {
		panic(fmt.Errorf("failed to unmarshal config: %w", err))
	}
	fmt.Printf("Config: %+v\n", conf)

	log.Init()
	//log.SetCaller(ctx, true)
	//log.SetFormatter(ctx, &logrus.JSONFormatter{
	//	TimestampFormat: "2006-01-02 15:04:05",
	//})
	ctx = log.WithFields(ctx, logrus.Fields{
		"app": "liveflow",
	})
	log.Info(ctx, "liveflow is started")
	hub := hub.NewHub()
	var tracks map[string][]*webrtc.TrackLocalStaticRTP
	tracks = make(map[string][]*webrtc.TrackLocalStaticRTP)
	// ingress
	// Egress 서비스는 streamID 알림을 구독하여 처리 시작
	go func() {
		api := echo.New()
		api.HideBanner = true
		hlsHub := hlshub.NewHLSHub()
		hlsHandler := httpsrv.NewHandler(hlsHub)
		api.GET("/hls/:streamID/master.m3u8", hlsHandler.HandleMasterM3U8)
		api.GET("/hls/:streamID/:playlistName/stream.m3u8", hlsHandler.HandleM3U8)
		api.GET("/hls/:streamID/:playlistName/:resourceName", hlsHandler.HandleM3U8)
		go func() {
			api.Start("0.0.0.0:" + strconv.Itoa(conf.HLS.Port))
		}()
		// ingress 의 rtmp, whip 서비스로부터 streamID를 받아 HLS, ContainerMP4, WHEP 서비스 시작
		for source := range hub.SubscribeToStreamID() {
			log.Infof(ctx, "New streamID received: %s", source.StreamID())
			hls := hls.NewHLS(hls.HLSArgs{
				Hub:    hub,
				HLSHub: hlsHub,
				Port:   conf.HLS.Port,
				LLHLS:  conf.HLS.LLHLS,
			})
			err := hls.Start(ctx, source)
			if err != nil {
				log.Errorf(ctx, "failed to start hls: %v", err)
			}
			mp4 := mp4.NewMP4(mp4.MP4Args{
				Hub: hub,
			})
			err = mp4.Start(ctx, source)
			if err != nil {
				log.Errorf(ctx, "failed to start mp4: %v", err)
			}
			whep := whep.NewWHEP(whep.WHEPArgs{
				Tracks: tracks,
				Hub:    hub,
			})
			err = whep.Start(ctx, source)
			if err != nil {
				log.Errorf(ctx, "failed to start whep: %v", err)
			}
			webmStarter := webm.NewWEBM(webm.WebMArgs{
				Hub: hub,
			})
			err = webmStarter.Start(ctx, source)
			if err != nil {
				log.Errorf(ctx, "failed to start webm: %v", err)
			}
		}
	}()

	whipServer := whip.NewWHIP(whip.WHIPArgs{
		Hub:        hub,
		Tracks:     tracks,
		DockerMode: conf.Docker.Mode,
		Port:       conf.Whep.Port,
	})
	go whipServer.Serve()
	rtmpServer := rtmp.NewRTMP(rtmp.RTMPArgs{
		Hub:  hub,
		Port: conf.RTMP.Port,
	})
	rtmpServer.Serve(ctx)
}
