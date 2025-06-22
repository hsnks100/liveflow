package httpsrv

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"liveflow/media/streamer/egress/thumbnail"
)

// ThumbnailHandler handles thumbnail HTTP requests
type ThumbnailHandler struct {
	store *thumbnail.ThumbnailStore
}

// NewThumbnailHandler creates a new thumbnail handler
func NewThumbnailHandler(store *thumbnail.ThumbnailStore) *ThumbnailHandler {
	return &ThumbnailHandler{
		store: store,
	}
}

// HandleThumbnail serves the latest thumbnail for a given stream ID
func (h *ThumbnailHandler) HandleThumbnail(c echo.Context) error {
	streamID := c.Param("streamID")

	// Fallback: parse URL path directly if Echo param extraction fails
	if streamID == "" {
		path := c.Request().URL.Path
		// Remove /thumbnail/ prefix and extract streamID
		if strings.HasPrefix(path, "/thumbnail/") {
			remaining := strings.TrimPrefix(path, "/thumbnail/")
			// Remove .jpg suffix if present
			if strings.HasSuffix(remaining, ".jpg") {
				remaining = strings.TrimSuffix(remaining, ".jpg")
			}
			streamID = remaining
		}
	}

	if streamID == "" {
		return c.JSON(http.StatusBadRequest, APIResponse{
			ErrorCode: 400,
			Message:   "stream ID is required",
		})
	}

	// Get thumbnail from memory store
	thumbnailData, exists := h.store.Get(streamID)
	if !exists {
		return c.JSON(http.StatusNotFound, APIResponse{
			ErrorCode: 404,
			Message:   fmt.Sprintf("no thumbnail found for stream %s", streamID),
		})
	}

	// Set appropriate headers
	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().Header().Set("Cache-Control", "max-age=30") // Cache for 30 seconds
	c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", len(thumbnailData.Data)))

	// Serve the thumbnail data
	return c.Blob(http.StatusOK, "image/jpeg", thumbnailData.Data)
}

// HandleThumbnailWithExtension serves thumbnail with .jpg extension in URL
func (h *ThumbnailHandler) HandleThumbnailWithExtension(c echo.Context) error {
	streamIDWithExt := c.Param("streamID")

	// Remove .jpg extension if present
	streamID := strings.TrimSuffix(streamIDWithExt, ".jpg")

	// Set the cleaned stream ID back to the context
	c.SetParamNames("streamID")
	c.SetParamValues(streamID)

	// Call the main handler
	return h.HandleThumbnail(c)
}
