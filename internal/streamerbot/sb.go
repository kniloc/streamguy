package streamerbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"log"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Event struct {
	Event     any             `json:"event"`
	Data      json.RawMessage `json:"data"`
	TimeStamp string          `json:"timeStamp"`
}

type EventHandler interface {
	HandleChatMessage(data json.RawMessage, timestamp string)
	HandleRewardRedemption(data json.RawMessage)
}

type Client struct {
	handler EventHandler
	conn    *websocket.Conn
	host    string
	port    string

	Connected    atomic.Bool
	Reconnecting atomic.Bool
}

const (
	wsURLFormat = "ws://%s:%s/"

	ReconnectDelay        = 5 * time.Second
	SubscriptionID        = "stream-guy-sub"
	TwitchChatEventName   = "Twitch.ChatMessage"
	TwitchRewardEventName = "Twitch.RewardRedemption"

	DefaultHexColorLength = 6
	DefaultColorAlpha     = 255
)

func FormatTimeStamp(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}
	return t.Format("3:04 PM")
}

func (e *Event) EventName() string {
	if str, ok := e.Event.(string); ok {
		return str
	}
	return e.extractEventNameFromMap()
}

func (e *Event) extractEventNameFromMap() string {
	eventMap, ok := e.Event.(map[string]any)
	if !ok {
		return ""
	}
	source, sourceOk := eventMap["source"].(string)
	eventType, typeOk := eventMap["type"].(string)
	if sourceOk && typeOk {
		return source + "." + eventType
	}
	return ""
}

func NewClient(handler EventHandler, host, port string) *Client {
	return &Client{
		handler: handler,
		host:    host,
		port:    port,
	}
}

func (c *Client) Connect() error {
	var err error

	url := fmt.Sprintf(wsURLFormat, c.host, c.port)
	log.Printf("Connecting to Streamer.bot at %s", url)

	c.conn, _, err = websocket.DefaultDialer.Dial(url, nil)

	if err != nil {
		c.Connected.Store(false)
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.Connected.Store(true)
	c.Reconnecting.Store(false)

	log.Println("Connected to Streamer.bot!")
	return nil
}

func (c *Client) Subscribe() error {
	subscribeMsg := map[string]any{
		"request": "Subscribe",
		"id":      SubscriptionID,
		"events": map[string][]string{
			"Twitch": {"ChatMessage", "RewardRedemption"},
		},
	}
	err := c.conn.WriteJSON(subscribeMsg)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	log.Println("Subscription request sent successfully")
	return nil
}

func (c *Client) Listen(ctx context.Context) {
	for {
		var event Event
		err := c.conn.ReadJSON(&event)

		if err != nil {
			if ctx != nil {
				select {
				case <-ctx.Done():
					c.Connected.Store(false)
					return
				default:
				}
			}
			if errors.Is(err, net.ErrClosed) {
				c.Connected.Store(false)
				return
			}
			log.Printf("Error reading message: %v", err)
			c.Connected.Store(false)
			return
		}

		eventName := event.EventName()

		if eventName == TwitchChatEventName {
			if c.handler != nil {
				c.handler.HandleChatMessage(event.Data, event.TimeStamp)
			}
		}

		if eventName == TwitchRewardEventName {
			if c.handler != nil {
				c.handler.HandleRewardRedemption(event.Data)
			}
		}
	}
}

func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
		c.Connected.Store(false)
	}
}

func (c *Client) Start(ctx context.Context) {
	if ctx != nil {
		go func() {
			<-ctx.Done()
			c.Close()
		}()
	}

	sleep := func(d time.Duration) bool {
		if ctx == nil {
			time.Sleep(d)
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
			return true
		}
	}

	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				c.Close()
				return
			default:
			}
		}

		c.Reconnecting.Store(true)

		if err := c.Connect(); err != nil {
			log.Printf("Failed to connect to Streamer.bot: %v", err)
			c.Connected.Store(false)
			if !sleep(ReconnectDelay) {
				return
			}
			continue
		}

		if err := c.Subscribe(); err != nil {
			log.Printf("Failed to subscribe: %v", err)
			c.Close()
			c.Connected.Store(false)
			if !sleep(ReconnectDelay) {
				return
			}
			continue
		}

		c.Listen(ctx)
		c.Close()

		if ctx != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
		}

		log.Println("Connection lost, reconnecting in 5 seconds...")
		c.Connected.Store(false)
		c.Reconnecting.Store(true)
		if !sleep(ReconnectDelay) {
			return
		}
	}
}

func ParseHexColor(hexColor string) color.NRGBA {
	defaultColor := color.NRGBA{R: 255, G: 255, B: 255, A: DefaultColorAlpha}
	if hexColor == "" {
		return defaultColor
	}
	hexColor = strings.TrimPrefix(hexColor, "#")
	if len(hexColor) != DefaultHexColorLength {
		return defaultColor
	}
	r, err1 := strconv.ParseUint(hexColor[0:2], 16, 8)
	g, err2 := strconv.ParseUint(hexColor[2:4], 16, 8)
	b, err3 := strconv.ParseUint(hexColor[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return defaultColor
	}
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: DefaultColorAlpha}
}
