package assets

import (
	"fmt"
	"image/gif"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const Directory = "assets"

type ImageCache struct {
	Gifs map[string]*gif.GIF
}

func NewImageCache() *ImageCache {
	return &ImageCache{
		Gifs: make(map[string]*gif.GIF),
	}
}

func (cache *ImageCache) LoadImages() {
	files, err := os.ReadDir(Directory)
	if err != nil {
		log.Printf("Error reading images directory: %v\n", err)
		return
	}

	for _, file := range files {
		if err := cache.tryLoadGifFile(file); err != nil {
			log.Printf("Failed to load GIF %s: %v\n", file.Name(), err)
		}
	}
}

func (cache *ImageCache) tryLoadGifFile(file os.DirEntry) (returnErr error) {
	if file.IsDir() {
		return nil
	}

	ext := filepath.Ext(file.Name())
	if ext != ".gif" {
		return nil
	}

	keyword := strings.TrimSuffix(file.Name(), ext)
	path := filepath.Join(Directory, file.Name())

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open GIF: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("failed to close GIF file: %w", closeErr)
		}
	}()

	gifImg, err := gif.DecodeAll(f)
	if err != nil {
		return fmt.Errorf("failed to decode GIF: %w", err)
	}

	cache.Gifs[keyword] = gifImg
	return nil
}

func ResolveKeyword(keyword string, keywords map[string]string) string {
	lowerKeyword := strings.ToLower(keyword)

	for k, v := range keywords {
		if strings.ToLower(k) == lowerKeyword {
			return v
		}
	}
	return keyword
}
