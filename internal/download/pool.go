package download

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/webp"
)

const (
	MaxDownloadSizeBytes = 5 * 1024 * 1024 // 5 MiB
	maxRetries           = 2
	retryBaseDelay       = 500 * time.Millisecond
)

// Magic byte signatures for image format detection.
var (
	magicGIF87 = []byte("GIF87a")
	magicGIF89 = []byte("GIF89a")
	magicPNG   = []byte{0x89, 0x50, 0x4E, 0x47}
	magicJPEG  = []byte{0xFF, 0xD8, 0xFF}
	magicWebP  = []byte("RIFF")
	magicWebP2 = []byte("WEBP") // bytes 8-11
)

type imageFormat int

const (
	formatUnknown imageFormat = iota
	formatGIF
	formatPNG
	formatJPEG
	formatWebP
)

type Pool struct {
	jobs       chan *Job
	done       chan struct{}
	numWorkers int
	client     *http.Client
	closeOnce  sync.Once
	wg         sync.WaitGroup
}

type Job struct {
	URL     string
	JobType string // "emote", "badge", or "photo"
	OnDone  func(result *Result)
}

type Result struct {
	URL         string
	GIF         *gif.GIF
	StaticImage image.Image
	IsGIF       bool
	ContentType string
	Error       error
}

func NewPool(numWorkers int) *Pool {
	pool := &Pool{
		jobs:       make(chan *Job, 100),
		done:       make(chan struct{}),
		numWorkers: numWorkers,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}

	for i := range numWorkers {
		pool.wg.Add(1)
		go pool.worker(i)
	}

	return pool
}

func (p *Pool) worker(_ int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.done:
			return
		case job := <-p.jobs:
			result := p.downloadWithRetry(job.URL)
			if job.OnDone != nil {
				job.OnDone(result)
			}
		}
	}
}

func (p *Pool) downloadWithRetry(url string) *Result {
	var result *Result
	for attempt := range maxRetries + 1 {
		result = p.downloadImage(url)
		if result.Error == nil || !isRetryable(result.Error) {
			return result
		}
		if attempt < maxRetries {
			delay := retryBaseDelay * time.Duration(1<<attempt)
			log.Printf("Retrying download (attempt %d/%d) for %s: %v", attempt+1, maxRetries, url, result.Error)
			select {
			case <-p.done:
				return result
			case <-time.After(delay):
			}
		}
	}
	return result
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "server error: ") {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	if strings.Contains(msg, "connection reset") || strings.Contains(msg, "EOF") {
		return true
	}
	return false
}

func (p *Pool) downloadImage(url string) *Result {
	result := &Result{URL: url}

	res, err := p.client.Get(url)
	if err != nil {
		result.Error = fmt.Errorf("failed to download: %w", err)
		return result
	}
	defer res.Body.Close()

	if res.StatusCode >= 500 {
		result.Error = fmt.Errorf("server error: %d", res.StatusCode)
		return result
	}

	if res.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("bad status: %d", res.StatusCode)
		return result
	}

	result.ContentType = res.Header.Get("Content-Type")

	if res.ContentLength > MaxDownloadSizeBytes {
		result.Error = fmt.Errorf("response too large: %d bytes", res.ContentLength)
		return result
	}

	limitedReader := io.LimitReader(res.Body, MaxDownloadSizeBytes+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		result.Error = fmt.Errorf("failed to read body: %w", err)
		return result
	}
	if int64(len(data)) > MaxDownloadSizeBytes {
		result.Error = fmt.Errorf("response too large: exceeded %d bytes", MaxDownloadSizeBytes)
		return result
	}

	format := detectFormat(data, result.ContentType)
	return decodeImage(result, data, format)
}

func detectFormat(data []byte, contentType string) imageFormat {
	if len(data) >= 6 && (bytes.Equal(data[:6], magicGIF87) || bytes.Equal(data[:6], magicGIF89)) {
		return formatGIF
	}
	if len(data) >= 4 && bytes.Equal(data[:4], magicPNG) {
		return formatPNG
	}
	if len(data) >= 3 && bytes.Equal(data[:3], magicJPEG) {
		return formatJPEG
	}
	if len(data) >= 12 && bytes.Equal(data[:4], magicWebP) && bytes.Equal(data[8:12], magicWebP2) {
		return formatWebP
	}

	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "image/gif"):
		return formatGIF
	case strings.Contains(ct, "image/png"):
		return formatPNG
	case strings.Contains(ct, "image/jpeg"):
		return formatJPEG
	case strings.Contains(ct, "image/webp"):
		return formatWebP
	}

	return formatUnknown
}

func decodeImage(result *Result, data []byte, format imageFormat) *Result {
	reader := bytes.NewReader(data)

	switch format {
	case formatGIF:
		gifImg, err := gif.DecodeAll(reader)
		if err != nil {
			result.Error = fmt.Errorf("failed to decode GIF: %w", err)
			return result
		}
		result.GIF = gifImg
		result.IsGIF = true
		return result

	case formatPNG:
		img, err := png.Decode(reader)
		if err != nil {
			result.Error = fmt.Errorf("failed to decode PNG: %w", err)
			return result
		}
		result.StaticImage = img
		return result

	case formatJPEG:
		img, err := jpeg.Decode(reader)
		if err != nil {
			result.Error = fmt.Errorf("failed to decode JPEG: %w", err)
			return result
		}
		result.StaticImage = img
		return result

	case formatWebP:
		img, err := webp.Decode(reader)
		if err != nil {
			result.Error = fmt.Errorf("failed to decode WebP: %w", err)
			return result
		}
		result.StaticImage = img
		return result

	default:
		// Fall back to trying GIF first, then generic image.Decode.
		gifImg, err := gif.DecodeAll(reader)
		if err == nil {
			result.GIF = gifImg
			result.IsGIF = true
			return result
		}

		reader.Reset(data)
		img, _, err := image.Decode(reader)
		if err != nil {
			result.Error = fmt.Errorf("failed to decode image: %w", err)
			return result
		}
		result.StaticImage = img
		return result
	}
}

func (p *Pool) Submit(url string, jobType string, onDone func(*Result)) {
	select {
	case <-p.done:
		return
	default:
	}

	job := &Job{
		URL:     url,
		JobType: jobType,
		OnDone:  onDone,
	}

	select {
	case <-p.done:
		return
	case p.jobs <- job:
	default:
		// Queue full: try to evict a lower-priority job.
		if jobType == "emote" || jobType == "photo" {
			select {
			case existing := <-p.jobs:
				if existing.JobType == "badge" {
					// Evict the badge job and enqueue the higher-priority one.
					log.Printf("Download queue full, evicting badge download for %s", existing.URL)
					select {
					case p.jobs <- job:
					default:
					}
				} else {
					// Put the existing job back and drop the new one.
					select {
					case p.jobs <- existing:
					default:
					}
					log.Printf("Warning: Download queue full, dropping download for %s", url)
				}
			default:
				log.Printf("Warning: Download queue full, dropping download for %s", url)
			}
		} else {
			log.Printf("Warning: Download queue full, dropping %s download for %s", jobType, url)
		}
	}
}

func (p *Pool) Close() {
	p.closeOnce.Do(func() {
		close(p.done)
	})
}

func (p *Pool) Wait() {
	p.wg.Wait()
}
