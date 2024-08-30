package main

import (
	"context"
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"

	"liveflow/httpsrv"
	"liveflow/log"
	"liveflow/media/hlshub"
	"liveflow/media/hub"
	"liveflow/media/streamer/egress/hls"
	"liveflow/media/streamer/egress/record/mp4"
	"liveflow/media/streamer/egress/whep"
	"liveflow/media/streamer/ingress/rtmp"
	"liveflow/media/streamer/ingress/whip"
)

// RTMP 받으면 자동으로 HLS 서비스 동작, 녹화 서비스까지~?
func main() {
	ctx := context.Background()
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
		api.GET("/health", func(c echo.Context) error {
			fmt.Println("hello")
			return c.String(200, "ok")
		})
		api.GET("/hls/:streamID/master.m3u8", hlsHandler.HandleMasterM3U8)
		api.GET("/hls/:streamID/:playlistName/stream.m3u8", hlsHandler.HandleM3U8)
		api.GET("/hls/:streamID/:playlistName/:resourceName", hlsHandler.HandleM3U8)
		go func() {
			api.Start("0.0.0.0:8044")
		}()
		for source := range hub.SubscribeToStreamID() {
			log.Infof(ctx, "New streamID received: %s", source.StreamID())
			hls := hls.NewHLS(hls.HLSArgs{
				Hub:    hub,
				HLSHub: hlsHub,
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
		}
	}()

	whipServer := whip.NewWHIP(whip.WHIPArgs{
		Hub:    hub,
		Tracks: tracks,
	})
	go whipServer.Serve()
	rtmpServer := rtmp.NewRTMP(rtmp.RTMPArgs{
		Hub: hub,
	})
	rtmpServer.Serve(ctx)
}
