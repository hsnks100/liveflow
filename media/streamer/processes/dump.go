package processes

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	astiav "liveflow/goastiav"
	"liveflow/media/streamer/pipe"
	"os"
)

type DumpProcess struct {
	pipe.BaseProcess[[]*astiav.Frame, interface{}]
	index int
	i     image.Image
}

func NewDumpProcess() *DumpProcess {
	return &DumpProcess{}
}

func (v *DumpProcess) Init() error {
	return nil
}
func (v *DumpProcess) Process(data []*astiav.Frame) (interface{}, error) {
	// Decode data
	ctx := context.Background()
	_ = ctx
	for _, frame := range data {
		filename := fmt.Sprintf("frame_%d.jpg", v.index)
		v.index++
		fd := frame.Data()
		if v.i == nil {
			var err error
			v.i, err = fd.GuessImageFormat()
			if err != nil {
				fmt.Println(err)
				return nil, err
			}
		}
		fd.ToImage(v.i)
		saveAsJPEG(v.i, filename)
	}

	return nil, nil
}
func saveAsJPEG(img image.Image, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	options := &jpeg.Options{
		Quality: 90, // JPEG 품질 (1~100)
	}
	return jpeg.Encode(file, img, options)
}
