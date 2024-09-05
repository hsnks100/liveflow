package main

import (
	"context"
	"fmt"
	"liveflow/media/streamer/egress/record/webm"

	"github.com/labstack/echo/v4"
	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"

	"liveflow/httpsrv"
	"liveflow/log"
	"liveflow/media/hlshub"
	"liveflow/media/hub"
	"liveflow/media/streamer/egress/hls"
	"liveflow/media/streamer/egress/whep"
	"liveflow/media/streamer/ingress/rtmp"
	"liveflow/media/streamer/ingress/whip"
)

//func main() {
//
//	var decCodec *astiav.Codec
//	var decCodecContext *astiav.CodecContext
//	var decFrame *astiav.Frame
//	decCodec = astiav.FindDecoder(astiav.CodecIDH264)
//	if decCodec == nil {
//		err := fmt.Errorf("main: codec is nil")
//		fmt.Println(err)
//	}
//	decCodecContext = astiav.AllocCodecContext(decCodec)
//	if decCodecContext == nil {
//		err := fmt.Errorf("main: codec context is nil")
//		fmt.Println(err)
//	}
//	if err := decCodecContext.Open(decCodec, nil); err != nil {
//		fmt.Println(err)
//	}
//	pkt := astiav.AllocPacket()
//	defer pkt.Free()
//
//	var h264Data []byte
//	file, err := os.Open("test.h264")
//	if err != nil {
//		fmt.Println(err)
//	}
//	defer file.Close()
//	h264Data, err = io.ReadAll(file)
//	if err != nil {
//		fmt.Println(err)
//	}
//	err = pkt.FromData(h264Data)
//	if err != nil {
//		fmt.Println(err)
//	}
//	err = decCodecContext.SendPacket(pkt)
//	if err != nil {
//		fmt.Println(err)
//	}
//	_ = decFrame
//}

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
		// ingress 의 rtmp, whip 서비스로부터 streamID를 받아 HLS, ContainerMP4, WHEP 서비스 시작
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
			//mp4 := mp4.NewMP4(mp4.MP4Args{
			//	Hub: hub,
			//})
			//err = mp4.Start(ctx, source)
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

			// aac -> opus
			//repeat := repeater.NewRepeater(repeater.RepeaterArgs{
			//	Hub: hub,
			//})
			//err = repeat.Start(ctx, source)
			//if err != nil {
			//	log.Errorf(ctx, "failed to start repeater: %v", err)
			//}
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
