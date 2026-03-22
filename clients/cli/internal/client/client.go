package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// Client is an HTTP client for the PicoClaw gateway REST API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates a Client for the given base URL and timeout.
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

// FromCommand creates a Client from cobra persistent flags and returns the JSON mode flag.
func FromCommand(cmd *cobra.Command) (*Client, bool, error) {
	gwURL, _ := cmd.Flags().GetString("gateway-url")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	jsonMode, _ := cmd.Flags().GetBool("json")
	return New(ResolveGatewayURL(gwURL), timeout), jsonMode, nil
}

// ResolveGatewayURL resolves the gateway URL from flag > env > default.
func ResolveGatewayURL(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("PICOCLAW_GATEWAY_URL"); env != "" {
		return env
	}
	return "http://localhost:8080"
}

// Health checks if the gateway is reachable.
func (c *Client) Health() error {
	_, err := c.get("/api/v1/status")
	return err
}

// GetStatus returns the gateway status.
func (c *Client) GetStatus() (*StatusResponse, error) {
	data, err := c.get("/api/v1/status")
	if err != nil {
		return nil, err
	}
	var resp StatusResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}
	return &resp, nil
}

// GetStatusRaw returns the raw JSON status response.
func (c *Client) GetStatusRaw() (json.RawMessage, error) {
	return c.get("/api/v1/status")
}

// ListChannels returns enabled channels.
func (c *Client) ListChannels() ([]ChannelInfo, error) {
	data, err := c.get("/api/v1/channels")
	if err != nil {
		return nil, err
	}
	var channels []ChannelInfo
	if err := json.Unmarshal(data, &channels); err != nil {
		return nil, fmt.Errorf("parse channels: %w", err)
	}
	return channels, nil
}

// ListAgents returns registered agents.
func (c *Client) ListAgents() ([]AgentInfo, error) {
	data, err := c.get("/api/v1/agents")
	if err != nil {
		return nil, err
	}
	var agents []AgentInfo
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, fmt.Errorf("parse agents: %w", err)
	}
	return agents, nil
}

// GetAgent returns a specific agent's details.
func (c *Client) GetAgent(id string) (*AgentDetail, error) {
	data, err := c.get("/api/v1/agents/" + id)
	if err != nil {
		return nil, err
	}
	var agent AgentDetail
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("parse agent: %w", err)
	}
	return &agent, nil
}

// ListTools returns available tools.
func (c *Client) ListTools() ([]ToolInfo, error) {
	data, err := c.get("/api/v1/tools")
	if err != nil {
		return nil, err
	}
	var tools []ToolInfo
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, fmt.Errorf("parse tools: %w", err)
	}
	return tools, nil
}

// ListSessions returns session summaries.
func (c *Client) ListSessions() ([]SessionListItem, error) {
	data, err := c.get("/api/v1/sessions")
	if err != nil {
		return nil, err
	}
	var sessions []SessionListItem
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("parse sessions: %w", err)
	}
	return sessions, nil
}

// GetSession returns a session's full message history.
func (c *Client) GetSession(id string) (*SessionDetail, error) {
	data, err := c.get("/api/v1/sessions/" + id)
	if err != nil {
		return nil, err
	}
	var session SessionDetail
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &session, nil
}

// DeleteSession deletes a session by ID.
func (c *Client) DeleteSession(id string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/api/v1/sessions/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return c.parseHTTPError(resp)
}

// GetConfig returns the current configuration.
func (c *Client) GetConfig() (json.RawMessage, error) {
	return c.get("/api/v1/config")
}

// PutConfig replaces the entire configuration.
func (c *Client) PutConfig(config json.RawMessage) error {
	return c.sendJSON(http.MethodPut, "/api/v1/config", config)
}

// PatchConfig applies a JSON merge patch to the configuration.
func (c *Client) PatchConfig(patch json.RawMessage) error {
	return c.sendJSON(http.MethodPatch, "/api/v1/config", patch)
}

// GetMetricsRaw fetches the raw Prometheus text from /metrics.
func (c *Client) GetMetricsRaw() ([]byte, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/metrics")
	if err != nil {
		return nil, fmt.Errorf("cannot connect to gateway at %s — is the gateway running?", c.BaseURL)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, c.parseHTTPErrorFromBody(resp.StatusCode, body)
	}
	return body, nil
}

func (c *Client) get(path string) (json.RawMessage, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + path)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to gateway at %s — is the gateway running?", c.BaseURL)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, c.parseHTTPErrorFromBody(resp.StatusCode, body)
	}
	return body, nil
}

func (c *Client) sendJSON(method, path string, payload json.RawMessage) error {
	req, err := http.NewRequest(method, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot connect to gateway at %s — is the gateway running?", c.BaseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return c.parseHTTPError(resp)
	}
	return nil
}

func (c *Client) parseHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return c.parseHTTPErrorFromBody(resp.StatusCode, body)
}

func (c *Client) parseHTTPErrorFromBody(statusCode int, body []byte) error {
	var errResp struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return fmt.Errorf("%s (HTTP %d)", errResp.Error, statusCode)
	}
	return fmt.Errorf("HTTP %d: %s", statusCode, http.StatusText(statusCode))
}
