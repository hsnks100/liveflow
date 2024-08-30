package httpsrv

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"path/filepath"

	"github.com/bluenviron/gohlslib/pkg/codecparams"
	"github.com/bluenviron/gohlslib/pkg/playlist"
	"github.com/labstack/echo/v4"

	"liveflow/log"
	"liveflow/media/hlshub"
)

const (
	cacheControl = "CDN-Cache-Control"
)

type Handler struct {
	endpoint *hlshub.HLSHub
}

func NewHandler(hlsEndpoint *hlshub.HLSHub) *Handler {
	return &Handler{
		endpoint: hlsEndpoint,
	}
}

func (h *Handler) HandleMasterM3U8(c echo.Context) error {
	fmt.Println("@@@ HandleMasterM3U8")
	workID := c.Param("streamID")
	muxers, err := h.endpoint.MuxersByWorkID(workID)
	if err != nil {
		fmt.Println("get muxer failed")
		return fmt.Errorf("get muxer failed: %w", err)
	}
	m3u8Version := 3
	pl := &playlist.Multivariant{
		Version: func() int {
			return m3u8Version
		}(),
		IndependentSegments: true,
	}
	var variants []*playlist.MultivariantVariant
	for name, muxer := range muxers {
		// TODO: muxer.Bandwidth() is not implemented
		//_, average, err := muxer.Bandwidth()
		//if err != nil {
		//	continue
		//}
		average := 33033
		variant := &playlist.MultivariantVariant{
			Bandwidth: average,
			FrameRate: nil,
			URI:       path.Join(name, "stream.m3u8"),
		}
		// TODO: muxer.ResolutionString() is not implemented
		//resolution, err := muxer.ResolutionString()
		//if err == nil {
		//	variant.Resolution = resolution
		//}
		variant.Codecs = []string{}
		if muxer.VideoTrack != nil {
			variant.Codecs = append(variant.Codecs, codecparams.Marshal(muxer.VideoTrack.Codec))
		}
		if muxer.AudioTrack != nil {
			variant.Codecs = append(variant.Codecs, codecparams.Marshal(muxer.AudioTrack.Codec))
		}
		variants = append(variants, variant)
	}
	pl.Variants = variants
	c.Response().Header().Set(cacheControl, "max-age=1")
	masterM3u8Bytes, err := pl.Marshal()
	if err != nil {
		return err
	}
	return c.Blob(http.StatusOK, "application/vnd.apple.mpegurl", masterM3u8Bytes)
}

func (h *Handler) HandleM3U8(c echo.Context) error {
	workID := c.Param("streamID")
	playlistName := c.Param("playlistName")
	ctx := context.Background()
	muxer, err := h.endpoint.Muxer(workID, playlistName)
	if err != nil {
		log.Error(ctx, err, "no hls stream")
		return c.NoContent(http.StatusNotFound)
	}
	extension := filepath.Ext(c.Request().URL.String())
	switch extension {
	case ".m3u8":
		c.Response().Header().Set(cacheControl, "max-age=1")
	case ".ts", ".mp4":
		c.Response().Header().Set(cacheControl, "max-age=3600")
	}
	muxer.Handle(c.Response(), c.Request())
	return nil
}
