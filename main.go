package main

import (
	"context"
	"fmt"

	"github.com/labstack/echo/v4"

	"mrw-clone/httpsrv"
	"mrw-clone/media/hlshub"
	"mrw-clone/media/hub"
	"mrw-clone/media/streamer/hls"
	"mrw-clone/media/streamer/record/mp4"
	"mrw-clone/media/streamer/rtmp"
)

// RTMP 받으면 자동으로 HLS 서비스 동작, 녹화 서비스까지~?
func main() {
	ctx := context.Background()

	hub := hub.NewHub()
	// ingress
	// Egress 서비스는 streamID 알림을 구독하여 처리 시작
	go func() {
		api := echo.New()
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

		for streamID := range hub.SubscribeToStreamID() {
			fmt.Printf("New streamID received: %s\n", streamID)

			hls := hls.NewHLS(hls.HLSArgs{
				Hub:    hub,
				HLSHub: hlsHub,
			})
			mp4 := mp4.NewMP4(mp4.MP4Args{
				Hub: hub,
			})
			hls.Start(ctx, streamID)
			mp4.Start(ctx, streamID)
		}
	}()

	rtmpServer := rtmp.NewRTMP(rtmp.RTMPArgs{
		Hub: hub,
	})
	rtmpServer.Serve(ctx)
}
