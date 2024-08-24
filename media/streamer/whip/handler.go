package whip

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/deepch/vdk/codec/h264parser"
	"github.com/labstack/echo/v4"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"

	"mrw-clone/log"
	"mrw-clone/media/hub"
)

type WHIP struct {
	hub *hub.Hub
}

type WHIPArgs struct {
	Hub *hub.Hub
}

var (
	videoTrack *webrtc.TrackLocalStaticRTP
	audioTrack *webrtc.TrackLocalStaticRTP

	peerConnectionConfiguration = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
)

func NewWHIP(args WHIPArgs) *WHIP {
	var err error
	if videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion"); err != nil {
		panic(err)
	}
	// Add Audio Track that is being written to from WHIP Session
	audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
	if err != nil {
		panic(err)
	}
	return &WHIP{
		hub: args.Hub,
	}
}

func (r *WHIP) Serve() {
	whipServer := echo.New()
	whipServer.Static("/", ".")
	whipServer.POST("/whip", whipHandler3)
	whipServer.POST("/whep", whepHandler3)
	//whipServer.PATCH("/whip", whipHandler)
	whipServer.Start(":5555")
}

func whipPatchHandler(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}
func whipHandler(c echo.Context) error {
	// Read the body of the request
	body, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to read request body")
	}

	// Print the received data
	fmt.Printf("Received WHIP request:\n%s\n", string(body))
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}
	// ICE Candidate가 발견되었을 때 호출될 콜백 설정
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			// 발견된 ICE Candidate를 시그널링 서버를 통해 다른 피어로 전송해야 합니다.
			fmt.Printf("New ICE Candidate found: %s\n", candidate.ToJSON().Candidate)
		}
	})
	// Offer SDP 받기
	var offerSDP string = string(body)

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}

	// Offer 처리
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}

	// Answer 생성
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// 로컬에 Answer 설정
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}

	// Video 트랙이 추가되었을 때 호출될 콜백 설정
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Track has started, of type %d\n", track.PayloadType())

		// 미디어 데이터를 처리하는 루프
		for {
			// RTP 패킷 수신
			rtpPacket, _, readErr := track.ReadRTP()
			if readErr != nil {
				panic(readErr)
			}

			// RTP 패킷 처리
			fmt.Printf("Received RTP packet with timestamp %d\n", rtpPacket.Timestamp)

			// 여기서 받은 RTP 데이터를 파일로 쓰거나 다른 곳으로 전송할 수 있습니다.
			// 예시: 비디오 데이터를 H.264 파일로 저장
			if track.Kind() == webrtc.RTPCodecTypeVideo {
				//saveToH264File("output.h264", rtpPacket.Payload)
			}
		}
	})

	// ICE 연결 상태 변경 이벤트 처리
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", state.String())
		if state == webrtc.ICEConnectionStateConnected {
			fmt.Println("ICE Connection Established!")
		}
	})
	// Answer SDP 출력
	fmt.Println("Generated SDP answer:")
	fmt.Println(answer.SDP)

	c.Response().Header().Set("Location", c.Request().URL.String())
	// Respond with a 200 OK
	return c.String(http.StatusCreated, answer.SDP)
}

func processFrame(ctx context.Context, packets []*rtp.Packet) error {
	var h264RTPParser = &codecs.H264Packet{}
	payload := make([]byte, 0)
	for _, pkt := range packets {
		b, err := h264RTPParser.Unmarshal(pkt.Payload)
		if err != nil {
			log.Error(ctx, err, "failed to unmarshal h264")
		}
		payload = append(payload, b...)
	}

	fmt.Println("h264 bytes len: ", len(payload))
	h264Bytes := payload
	//muxer.Write(time.Now(), time.Duration(timestamp)*time.Millisecond, h264Bytes, videoIndex)
	if len(h264Bytes) > 0 {
		au, _ := h264parser.SplitNALUs(h264Bytes)
		for _, nalu := range au {
			sliceType, _ := h264parser.ParseSliceHeaderFromNALU(nalu)
			nalUnitType := nalu[0] & 0x1f
			switch nalUnitType {
			case h264parser.NALU_SPS, h264parser.NALU_PPS:
				// SPS와 PPS는 이미 처리됨
				fmt.Println("SPS PPS")
			default:
				fmt.Println("sliceType: ", sliceType, "nalUnitType: ", nalUnitType)
			}
		}
	}
	return nil
}
func whipHandler3(c echo.Context) error {
	// Read the offer from HTTP Request
	offer, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Create a MediaEngine object to configure the supported codec
	m := &webrtc.MediaEngine{}

	// Setup the codecs you want to use.
	if err = m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, Channels: 0, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	if err = m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Create a InterceptorRegistry
	i := &interceptor.Registry{}

	// Register a intervalpli factory
	intervalPliFactory, err := intervalpli.NewReceiverInterceptor()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	i.Add(intervalPliFactory)

	// Use the default set of Interceptors
	if err = webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Create the API object with the MediaEngine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))

	// Create a new RTCPeerConnection
	peerConnection, err := api.NewPeerConnection(peerConnectionConfiguration)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Allow us to receive 1 video track
	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	// Allow us to receive 1 audio track
	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	ctx := context.Background()
	// Set a handler for when a new remote track starts
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Track has started, of type %s\n", track.Kind())
		var packets []*rtp.Packet
		currentTimestamp := uint32(0)
		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				log.Error(ctx, err, "failed to read rtp")
				break
			}

			switch track.Kind() {
			case webrtc.RTPCodecTypeVideo:
				//fmt.Println("timestamp: ", pkt.Timestamp)
				if len(packets) > 0 && currentTimestamp != pkt.Timestamp {
					fmt.Println("by timestamp: ", pkt.Timestamp)
					//processFrame(ctx, packets) // , muxer, videoIndex, audioIndex, timestampGen.GetTimestamp(packets[0].Timestamp))
					packets = nil
				}

				packets = append(packets, pkt)
				currentTimestamp = pkt.Timestamp
				if pkt.Marker {
					fmt.Println("by marker: ", pkt.Timestamp)
					processFrame(ctx, packets) // , muxer, videoIndex, audioIndex, timestampGen.GetTimestamp(packets[0].Timestamp))
					packets = nil
				}

				//fmt.Println("frame len: ", len(h264Bytes))
				if err = videoTrack.WriteRTP(pkt); err != nil {
					panic(err)
				}
			case webrtc.RTPCodecTypeAudio:
				if err = audioTrack.WriteRTP(pkt); err != nil {
					panic(err)
				}
			}
		}
		//err = muxer.WriteTrailer()
		//if err != nil {
		//	panic(err)
		//}
	})

	// Send answer via HTTP Response
	return writeAnswer3(c, peerConnection, offer, "/whip")
}

func writeAnswer3(c echo.Context, peerConnection *webrtc.PeerConnection, offer []byte, path string) error {
	// Set the handler for ICE connection state
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateFailed {
			_ = peerConnection.Close()
		}
	})

	if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: string(offer)}); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	} else if err = peerConnection.SetLocalDescription(answer); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	<-gatherComplete

	// WHIP+WHEP expects a Location header and a HTTP Status Code of 201
	c.Response().Header().Add("Location", path)
	c.Response().WriteHeader(http.StatusCreated)

	// Write Answer with Candidates as HTTP Response
	return c.String(http.StatusOK, peerConnection.LocalDescription().SDP)
}

func whepHandler3(c echo.Context) error {
	// Read the offer from HTTP Request
	offer, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewPeerConnection(peerConnectionConfiguration)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Add Video Track that is being written to from WHIP Session
	rtpSender, err := peerConnection.AddTrack(videoTrack)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	rtpSenderAudio, err := peerConnection.AddTrack(audioTrack)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Read incoming RTCP packets
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()
	// Read incoming RTCP packets for audio
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSenderAudio.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	// Send answer via HTTP Response
	return writeAnswer3(c, peerConnection, offer, "/whep")
}
