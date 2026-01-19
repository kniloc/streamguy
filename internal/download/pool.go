package download

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	MaxDownloadSizeBytes = 5 * 1024 * 1024 // 5 MiB
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
	JobType string // "emote" or "badge"
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
			result := p.downloadImage(job.URL)
			if job.OnDone != nil {
				job.OnDone(result)
			}
		}
	}
}

func (p *Pool) downloadImage(url string) *Result {
	result := &Result{URL: url}

	res, err := p.client.Get(url)
	if err != nil {
		result.Error = fmt.Errorf("failed to download: %w", err)
		return result
	}
	defer res.Body.Close()

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

	gifImg, err := gif.DecodeAll(bytes.NewReader(data))
	if err == nil {
		result.GIF = gifImg
		result.IsGIF = true
		return result
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		result.Error = fmt.Errorf("failed to decode image: %w", err)
		return result
	}

	result.StaticImage = img
	result.IsGIF = false
	return result
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
		log.Printf("Warning: Download queue full, dropping download for %s", url)
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
