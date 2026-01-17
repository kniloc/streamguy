package assets

import (
	"fmt"
	"image"
	"log"
	"sync"

	"stream-guy/internal/download"

	"gioui.org/app"
)

var ErrAssetDownloading = &simpleError{msg: "asset is downloading"}

type simpleError struct {
	msg string
}

func (e *simpleError) Error() string {
	return e.msg
}

type BaseManager struct {
	downloadPool *download.Pool
	cache        map[string]image.Image
	cacheMu      sync.RWMutex
	downloading  map[string]bool
	downloadMu   sync.RWMutex
	Windows      map[string][]*app.Window
	windowsMu    sync.RWMutex
}

func NewBaseManager(downloadPool *download.Pool) *BaseManager {
	return &BaseManager{
		downloadPool: downloadPool,
		cache:        make(map[string]image.Image),
		downloading:  make(map[string]bool),
		Windows:      make(map[string][]*app.Window),
	}
}

func (bam *BaseManager) RegisterWindow(url string, window *app.Window) {
	bam.windowsMu.Lock()
	defer bam.windowsMu.Unlock()
	bam.Windows[url] = append(bam.Windows[url], window)
}

func (bam *BaseManager) UnregisterWindow(window *app.Window) {
	bam.windowsMu.Lock()
	defer bam.windowsMu.Unlock()

	for url, windows := range bam.Windows {
		filtered := make([]*app.Window, 0, len(windows))
		for _, w := range windows {
			if w != window {
				filtered = append(filtered, w)
			}
		}
		bam.Windows[url] = filtered
	}
}

func (bam *BaseManager) GetCached(url string) (image.Image, bool) {
	bam.cacheMu.RLock()
	defer bam.cacheMu.RUnlock()
	img, ok := bam.cache[url]
	return img, ok
}

func (bam *BaseManager) SetCached(url string, img image.Image) {
	bam.cacheMu.Lock()
	defer bam.cacheMu.Unlock()
	bam.cache[url] = img
}

func (bam *BaseManager) IsDownloading(url string) bool {
	bam.downloadMu.RLock()
	defer bam.downloadMu.RUnlock()
	return bam.downloading[url]
}

func (bam *BaseManager) MarkDownloading(url string, downloading bool) {
	bam.downloadMu.Lock()
	defer bam.downloadMu.Unlock()
	if downloading {
		bam.downloading[url] = true
	} else {
		delete(bam.downloading, url)
	}
}

func (bam *BaseManager) InvalidateWindows(url string) {
	bam.windowsMu.RLock()
	windows := bam.Windows[url]
	bam.windowsMu.RUnlock()

	for _, w := range windows {
		if w != nil {
			w.Invalidate()
		}
	}
}

func (bam *BaseManager) DownloadAsset(url string, jobType string, processor func(*download.Result) error) error {
	if bam.IsDownloading(url) {
		return ErrAssetDownloading
	}

	bam.MarkDownloading(url, true)

	if bam.downloadPool == nil {
		bam.MarkDownloading(url, false)
		log.Printf("Download pool not initialized; dropping download for %s", url)
		return fmt.Errorf("download pool not initialized")
	}
	bam.downloadPool.Submit(url, jobType, func(result *download.Result) {
		bam.MarkDownloading(url, false)

		if result.Error != nil {
			log.Printf("Failed to download %s %s: %v", jobType, url, result.Error)
			return
		}

		if err := processor(result); err != nil {
			log.Printf("Failed to process %s %s: %v", jobType, url, err)
			return
		}

		bam.InvalidateWindows(url)
	})

	return ErrAssetDownloading
}
