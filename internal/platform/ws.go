package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"llm-share/agent/internal/config"
	"llm-share/agent/internal/runtime"
)

const agentVersion = "0.1.0"

type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type Offering struct {
	ModelID               string  `json:"model_id"`
	PriceInputPerMillion  float64 `json:"price_input_per_million"`
	PriceOutputPerMillion float64 `json:"price_output_per_million"`
}

type RegisterData struct {
	Runtime    string     `json:"runtime"`
	RuntimeURL string     `json:"runtime_url"`
	Offerings  []Offering `json:"offerings"`
}

type JobData struct {
	JobID    string          `json:"job_id"`
	Model    string          `json:"model"`
	Messages json.RawMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ChunkData struct {
	JobID string `json:"job_id"`
	Delta string `json:"delta"`
}

type CompleteData struct {
	JobID     string `json:"job_id"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
}

type ErrorData struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
}

type Client struct {
	cfg     config.Config
	runtime runtime.Client
	dialer  *websocket.Dialer
	log     *log.Logger
}

func NewClient(cfg config.Config, rt runtime.Client, logger *log.Logger) *Client {
	if logger == nil {
		logger = log.Default()
	}
	return &Client{
		cfg:     cfg,
		runtime: rt,
		dialer:  websocket.DefaultDialer,
		log:     logger,
	}
}

func (c *Client) Run(ctx context.Context) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := c.sendRegister(conn); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	c.log.Printf("registered with platform as %s runtime", c.cfg.Runtime)

	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()

	var writeMu sync.Mutex
	go c.heartbeatLoop(heartbeatCtx, conn, &writeMu)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}

		var env Envelope
		if err := json.Unmarshal(msg, &env); err != nil {
			c.log.Printf("invalid envelope: %v", err)
			continue
		}

		switch env.Type {
		case "job":
			var job JobData
			if err := json.Unmarshal(env.Data, &job); err != nil {
				c.log.Printf("invalid job payload: %v", err)
				continue
			}
			go c.handleJob(ctx, conn, &writeMu, job)
		default:
			c.log.Printf("ignored message type: %s", env.Type)
		}
	}
}

func (c *Client) connect() (*websocket.Conn, error) {
	u, err := url.Parse(c.cfg.PlatformURL)
	if err != nil {
		return nil, fmt.Errorf("parse platform_url: %w", err)
	}

	q := u.Query()
	q.Set("token", c.cfg.ProviderToken)
	u.RawQuery = q.Encode()

	header := http.Header{}
	header.Set("User-Agent", "llm-share-agent/"+agentVersion)

	conn, _, err := c.dialer.Dial(u.String(), header)
	if err != nil {
		return nil, fmt.Errorf("dial platform: %w", err)
	}
	return conn, nil
}

func (c *Client) sendRegister(conn *websocket.Conn) error {
	offerings := make([]Offering, 0, len(c.cfg.Models))
	for _, m := range c.cfg.Models {
		offerings = append(offerings, Offering{
			ModelID:               m.ID,
			PriceInputPerMillion:  m.PriceInputPerMillion,
			PriceOutputPerMillion: m.PriceOutputPerMillion,
		})
	}

	return c.write(conn, nil, Envelope{
		Type: "register",
		Data: mustMarshal(RegisterData{
			Runtime:    c.cfg.Runtime,
			RuntimeURL: c.cfg.RuntimeURL,
			Offerings:  offerings,
		}),
	})
}

func (c *Client) heartbeatLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex) {
	ticker := time.NewTicker(c.cfg.HeartbeatInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.write(conn, writeMu, Envelope{Type: "heartbeat", Data: json.RawMessage(`{}`)}); err != nil {
				c.log.Printf("heartbeat failed: %v", err)
				return
			}
		}
	}
}

func (c *Client) handleJob(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, job JobData) {
	var messages []runtime.ChatMessage
	if err := json.Unmarshal(job.Messages, &messages); err != nil {
		c.sendError(conn, writeMu, job.JobID, fmt.Sprintf("invalid messages: %v", err))
		return
	}

	tokensIn, tokensOut, err := c.runtime.Chat(ctx, job.Model, messages, job.Stream, func(delta string) error {
		return c.write(conn, writeMu, Envelope{
			Type: "chunk",
			Data: mustMarshal(ChunkData{
				JobID: job.JobID,
				Delta: delta,
			}),
		})
	})
	if err != nil {
		c.sendError(conn, writeMu, job.JobID, err.Error())
		return
	}

	if err := c.write(conn, writeMu, Envelope{
		Type: "complete",
		Data: mustMarshal(CompleteData{
			JobID:     job.JobID,
			TokensIn:  tokensIn,
			TokensOut: tokensOut,
		}),
	}); err != nil {
		c.log.Printf("complete failed for job %s: %v", job.JobID, err)
	}
}

func (c *Client) sendError(conn *websocket.Conn, writeMu *sync.Mutex, jobID, message string) {
	if err := c.write(conn, writeMu, Envelope{
		Type: "error",
		Data: mustMarshal(ErrorData{
			JobID:   jobID,
			Message: message,
		}),
	}); err != nil {
		c.log.Printf("error message failed for job %s: %v", jobID, err)
	}
}

func (c *Client) write(conn *websocket.Conn, writeMu *sync.Mutex, env Envelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}

	if writeMu != nil {
		writeMu.Lock()
		defer writeMu.Unlock()
	}

	return conn.WriteMessage(websocket.TextMessage, payload)
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
