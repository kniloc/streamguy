package assets

import (
	"fmt"
	"image"

	"stream-guy/internal/download"

	"gioui.org/app"
)

const BadgeSize = 18

type BadgeManager struct {
	*BaseManager
}

func NewBadgeManager(downloadPool *download.Pool) *BadgeManager {
	return &BadgeManager{
		BaseManager: NewBaseManager(downloadPool),
	}
}

func (bm *BadgeManager) GetBadge(url string, window *app.Window) (image.Image, error) {
	bm.RegisterWindow(url, window)

	if img, ok := bm.GetCached(url); ok {
		return img, nil
	}

	if bm.IsDownloading(url) {
		return nil, ErrAssetDownloading
	}

	return nil, bm.downloadBadge(url)
}

func (bm *BadgeManager) Prefetch(url string) {
	if _, ok := bm.GetCached(url); ok {
		return
	}

	if bm.IsDownloading(url) {
		return
	}

	bm.downloadBadge(url)
}

func (bm *BadgeManager) downloadBadge(url string) error {
	return bm.DownloadAsset(url, "badge", func(result *download.Result) error {
		var img image.Image
		if result.IsGIF && result.GIF != nil && len(result.GIF.Image) > 0 {
			img = result.GIF.Image[0]
		} else {
			img = result.StaticImage
		}

		if img == nil {
			return fmt.Errorf("no badge image data")
		}

		scaled := ScaleImage(img, BadgeSize, BadgeSize)
		bm.SetCached(url, scaled)

		return nil
	})
}
