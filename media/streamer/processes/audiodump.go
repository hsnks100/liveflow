package processes

import (
	"context"
	"image"
	"liveflow/media/streamer/pipe"

	astiav "github.com/asticode/go-astiav"
)

type AudioDumpProcess struct {
	pipe.BaseProcess[[]*astiav.Frame, interface{}]
	index int
	i     image.Image
}

func NewAudioDumpProcess() *AudioDumpProcess {
	return &AudioDumpProcess{}
}

func (v *AudioDumpProcess) Init() error {
	return nil
}
func (v *AudioDumpProcess) Process(data []*astiav.Frame) (interface{}, error) {
	// Decode data
	ctx := context.Background()
	_ = ctx

	return nil, nil
}
