package rtmp

import (
	"context"
	"io"
	"net"

	"github.com/yutopp/go-rtmp"

	"mrw-clone/log"
	"mrw-clone/media/hub"
)

const (
	aacDefaultSampleRate = 44100
)

type RTMP struct {
	serverConfig *rtmp.ServerConfig
	hub          *hub.Hub
}

type RTMPArgs struct {
	ServerConfig *rtmp.ServerConfig
	Hub          *hub.Hub
}

func NewRTMP(args RTMPArgs) *RTMP {
	return &RTMP{
		//serverConfig: args.ServerConfig,
		hub: args.Hub,
	}
}

func (r *RTMP) Serve(ctx context.Context) error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", ":1930")
	if err != nil {
		log.Errorf(ctx, "Failed: %+v", err)
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Errorf(ctx, "Failed: %+v", err)
	}
	srv := rtmp.NewServer(&rtmp.ServerConfig{
		OnConnect: func(conn net.Conn) (io.ReadWriteCloser, *rtmp.ConnConfig) {
			h := &Handler{
				hub: r.hub,
			}
			return conn, &rtmp.ConnConfig{
				Handler: h,
				//ControlState: rtmp.StreamControlStateConfig{
				//	DefaultBandwidthWindowSize: 6 * 1024 * 1024 / 8,
				//},
				//Logger: nil,
				SkipHandshakeVerification:               false,
				IgnoreMessagesOnNotExistStream:          false,
				IgnoreMessagesOnNotExistStreamThreshold: 0,
				ReaderBufferSize:                        0,
				WriterBufferSize:                        0,
				ControlState:                            rtmp.StreamControlStateConfig{DefaultChunkSize: 0, MaxChunkSize: 0, MaxChunkStreams: 0, DefaultAckWindowSize: 0, MaxAckWindowSize: 0, DefaultBandwidthWindowSize: 6 * 1024 * 1024 / 8, DefaultBandwidthLimitType: 0, MaxBandwidthWindowSize: 0, MaxMessageSize: 0, MaxMessageStreams: 0},
				Logger:                                  nil,
				RPreset:                                 nil,
			}
		},
	})
	if err := srv.Serve(listener); err != nil {
		log.Errorf(ctx, "Failed: %+v", err)
	}
	return nil
}
