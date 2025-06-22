package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // pprof을 사용하기 위한 패키지
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"liveflow/config"
	"liveflow/media/streamer/egress/hls"
	"liveflow/media/streamer/egress/record/mp4"
	"liveflow/media/streamer/egress/record/webm"
	"liveflow/media/streamer/egress/whep"
	"liveflow/media/streamer/ingress/whip"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"liveflow/httpsrv"
	"liveflow/log"
	"liveflow/media/hlshub"
	"liveflow/media/hub"
	"liveflow/media/streamer/egress/thumbnail"
	"liveflow/media/streamer/ingress/rtmp"
)

/*
#include <stdio.h>
#include <stdlib.h>
void __lsan_do_leak_check(void);
void leak_bit() {
	int *p = (int *)malloc(sizeof(int));
}
*/
import "C"

// RTMP 받으면 자동으로 Service 서비스 동작, 녹화 서비스까지~?
func main() {
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
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Info(ctx, "Received signal:", sig)
		log.Info(ctx, "Initiating shutdown...")
		//C.__lsan_do_leak_check()
		cancel()
	}()
	ctx = log.WithFields(ctx, logrus.Fields{
		"app": "liveflow",
	})
	log.Info(ctx, "liveflow is started")
	sourceHub := hub.NewHub()
	var tracks map[string][]*webrtc.TrackLocalStaticRTP
	tracks = make(map[string][]*webrtc.TrackLocalStaticRTP)
	// Egress is started by streamID notification
	hlsHub := hlshub.NewHLSHub()
	go func() {
		api := echo.New()
		api.HideBanner = true
		hlsHandler := httpsrv.NewHandler(hlsHub)
		api.Use(middleware.Logger())

		// 1. API routes
		hlsRoute := api.Group("/hls", middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: []string{"*"}, // Adjust origins as necessary
			AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodOptions},
		}))
		hlsRoute.GET("/:streamID/master.m3u8", hlsHandler.HandleMasterM3U8)
		hlsRoute.GET("/:streamID/:playlistName/stream.m3u8", hlsHandler.HandleM3U8)
		hlsRoute.GET("/:streamID/:playlistName/:resourceName", hlsHandler.HandleM3U8)
		api.GET("/streams", hlsHandler.HandleListStreams)

		whipServer := whip.NewWHIP(whip.WHIPArgs{
			Hub:        sourceHub,
			Tracks:     tracks,
			DockerMode: conf.Docker.Mode,
			Echo:       api,
		})
		whipServer.RegisterRoute()

		// 2. Serve static files
		api.Static("/", "front/dist")

		// 3. SPA fallback for all other routes
		api.GET("/*", func(c echo.Context) error {
			return c.File("front/dist/index.html")
		})

		go func() {
			fmt.Println("----------------", conf.Service.Port)
			api.Start("0.0.0.0:" + strconv.Itoa(conf.Service.Port))
		}()

		type Starter interface {
			Start(ctx context.Context, source hub.Source) error
			Name() string
		}

		// ingress 의 rtmp, whip 서비스로부터 streamID를 받아 Service, ContainerMP4, WHEP 서비스 시작
		for source := range sourceHub.SubscribeToStreamID() {
			log.Infof(ctx, "New streamID received: %s", source.StreamID())

			var starters []Starter

			if conf.MP4.Record {
				starters = append(starters, mp4.NewMP4(mp4.MP4Args{
					Hub:             sourceHub,
					SplitIntervalMS: 3000,
				}))
			}

			if conf.EBML.Record {
				starters = append(starters, webm.NewWEBM(webm.WebMArgs{
					Hub:             sourceHub,
					SplitIntervalMS: 6000,
					StreamID:        source.StreamID(),
				}))
			}

			if conf.Thumbnail.Enable {
				starters = append(starters, thumbnail.NewThumbnail(thumbnail.ThumbnailArgs{
					Hub:             sourceHub,
					OutputPath:      conf.Thumbnail.OutputPath,
					IntervalSeconds: conf.Thumbnail.IntervalSeconds,
					Width:           conf.Thumbnail.Width,
					Height:          conf.Thumbnail.Height,
				}))
			}

			starters = append(starters, hls.NewHLS(hls.HLSArgs{
				Hub:     sourceHub,
				HLSHub:  hlsHub,
				Port:    conf.Service.Port,
				LLHLS:   conf.Service.LLHLS,
				DiskRam: conf.Service.DiskRam,
			}))

			starters = append(starters, whep.NewWHEP(whep.WHEPArgs{
				Tracks: tracks,
				Hub:    sourceHub,
			}))

			for _, starter := range starters {
				if err := starter.Start(ctx, source); err != nil {
					log.Errorf(ctx, "failed to start %s for stream %s: %v", starter.Name(), source.StreamID(), err)
				}
			}
		}
	}()

	rtmpServer := rtmp.NewRTMP(rtmp.RTMPArgs{
		Hub:    sourceHub,
		Port:   conf.RTMP.Port,
		HLSHub: hlsHub,
	})
	rtmpServer.Serve(ctx)
}
