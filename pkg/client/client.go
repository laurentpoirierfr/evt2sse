// Package client fournit un client Go pour interroger un serveur evt2sse.
//
// Il permet d'écouter le flux Server-Sent Events (GET /api/listen) et
// d'émettre des notifications PostgreSQL (POST /api/send).
//
// Exemple :
//
//	cli := client.New("http://localhost:8080")
//	stream := cli.Listen(context.Background())
//	go func() {
//		for evt := range stream.Events() {
//			fmt.Println(evt.Channel, evt.Payload)
//		}
//	}()
//	_ = cli.Send(context.Background(), "evt2sse", `{"hello":"world"}`)
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultChannel = "evt2sse"

// Event est une notification reçue du flux SSE.
type Event struct {
	ID      int64     `json:"id"`
	Channel string    `json:"channel"`
	Payload string    `json:"payload"`
	Time    time.Time `json:"time"`
}

// Status reflète l'état du serveur (GET /api/status).
type Status struct {
	Channel   string `json:"channel"`
	Clients   int    `json:"clients"`
	LastID    int64  `json:"last_id"`
	Connected bool   `json:"connected"`
}

// Client est un client HTTP vers un serveur evt2sse.
type Client struct {
	baseURL   string
	channel   string
	http      *http.Client
	reconnect time.Duration
}

// Option configure un Client.
type Option func(*Client)

// WithHTTPClient remplace le client HTTP utilisé en interne.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// WithDefaultChannel définit le canal utilisé par Send quand aucun canal
// n'est précisé (défaut : evt2sse).
func WithDefaultChannel(ch string) Option {
	return func(c *Client) { c.channel = ch }
}

// New construit un client pointant vers baseURL (ex. http://localhost:8080).
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		channel:   defaultChannel,
		http:      &http.Client{Timeout: 30 * time.Second},
		reconnect: 3 * time.Second,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type sendRequest struct {
	Channel string `json:"channel,omitempty"`
	Payload string `json:"payload"`
}

type sendResponse struct {
	OK      bool   `json:"ok"`
	Channel string `json:"channel"`
	Payload string `json:"payload"`
	Error   string `json:"error,omitempty"`
}

// Send publie une notification via POST /api/send. Le canal est transmis tel
// quel (quelle que soit sa valeur, le serveur relaie sur son canal LISTEN).
func (c *Client) Send(ctx context.Context, channel, payload string) error {
	if channel == "" {
		channel = c.channel
	}
	body, err := json.Marshal(sendRequest{Channel: channel, Payload: payload})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out sendResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("send: réponse invalide: %w", err)
	}
	if resp.StatusCode != http.StatusOK || !out.OK {
		return fmt.Errorf("send: %s", errText(resp.StatusCode, out.Error))
	}
	return nil
}

// SendJSON sérialise v en JSON et le publie comme payload de notification.
func (c *Client) SendJSON(ctx context.Context, channel string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Send(ctx, channel, string(b))
}

// Status interroge GET /api/status.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/status", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("status: statut HTTP %d", resp.StatusCode)
	}
	var s Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

type channelsResponse struct {
	Default  string   `json:"default"`
	Channels []string `json:"channels"`
}

type channelRequest struct {
	Channel string `json:"channel"`
}

type channelResponse struct {
	OK      bool   `json:"ok"`
	Channel string `json:"channel,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Channels liste les canaux actuellement écoutés par le serveur.
func (c *Client) Channels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/channels", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("channels: statut HTTP %d", resp.StatusCode)
	}
	var out channelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Channels, nil
}

// Subscribe active l'écoute serveur d'un canal (LISTEN PostgreSQL).
func (c *Client) Subscribe(ctx context.Context, channel string) error {
	return c.channelOp(ctx, http.MethodPost, "", channel)
}

// Unsubscribe ferme l'écoute serveur d'un canal.
func (c *Client) Unsubscribe(ctx context.Context, channel string) error {
	return c.channelOp(ctx, http.MethodDelete, url.PathEscape(channel), "")
}

func (c *Client) channelOp(ctx context.Context, method, escapedPath, channel string) error {
	body, err := json.Marshal(channelRequest{Channel: channel})
	if err != nil {
		return err
	}
	url := c.baseURL + "/api/channels"
	if escapedPath != "" {
		url += "/" + escapedPath
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out channelResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("channel: réponse invalide: %w", err)
	}
	if resp.StatusCode != http.StatusOK || !out.OK {
		return fmt.Errorf("channel %q: %s", channel, errText(resp.StatusCode, out.Error))
	}
	return nil
}

func errText(code int, msg string) string {
	if msg != "" {
		return msg
	}
	return fmt.Sprintf("statut HTTP %d", code)
}
