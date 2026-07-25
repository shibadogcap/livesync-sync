// Package couchdb provides a lightweight CouchDB HTTP client.
// This is a clean-room implementation (no PouchDB dependency)
// that handles Basic auth, changes feed, and document CRUD.
package couchdb

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a lightweight CouchDB HTTP client.
type Client struct {
	baseURL  string
	database string
	username string
	password string
	client   *http.Client
}

// NewClient creates a new CouchDB client.
func NewClient(baseURL, database, username, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		database: database,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

// dbURL returns the URL for the database.
func (c *Client) dbURL() string {
	return c.baseURL + "/" + c.database
}

// docURL returns the URL for a specific document.
func (c *Client) docURL(id string) string {
	return c.dbURL() + "/" + urlEncodeDocID(id)
}

// localDocURL returns the URL for a local document.
func (c *Client) localDocURL(id string) string {
	return c.dbURL() + "/" + id
}

// setAuth adds Basic authentication headers to a request.
func (c *Client) setAuth(req *http.Request) {
	auth := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
	req.Header.Set("Authorization", "Basic "+auth)
}

// Probe checks if the CouchDB server is reachable and the database exists.
func (c *Client) Probe() error {
	// Check server with auth
	resp, err := c.request("GET", c.baseURL+"/", nil)
	if err != nil {
		return fmt.Errorf("server unreachable: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d (auth required?)", resp.StatusCode)
	}

	// Check database exists
	resp2, err := c.request("GET", c.dbURL(), nil)
	if err != nil {
		return fmt.Errorf("database unreachable: %w", err)
	}
	resp2.Body.Close()

	if resp2.StatusCode == http.StatusNotFound {
		return fmt.Errorf("database '%s' not found", c.database)
	}
	if resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("database check returned %d", resp2.StatusCode)
	}

	return nil
}

// request performs an HTTP request with auth headers.
func (c *Client) request(method, url string, body []byte) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.client.Do(req)
}

// GetDoc retrieves a document by ID.
// Returns the raw JSON bytes and the response.
func (c *Client) GetDoc(id string) ([]byte, error) {
	resp, err := c.request("GET", c.docURL(id), nil)
	if err != nil {
		return nil, fmt.Errorf("GET doc failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Document not found, not an error
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET doc %s returned %d: %s", id, resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// GetLocalDoc retrieves a local document (starts with _local/).
func (c *Client) GetLocalDoc(id string) ([]byte, error) {
	resp, err := c.request("GET", c.localDocURL(id), nil)
	if err != nil {
		return nil, fmt.Errorf("GET local doc failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET local doc %s returned %d: %s", id, resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// PutDoc creates or updates a document.
// Returns the new revision (_rev).
func (c *Client) PutDoc(id string, data []byte) (string, error) {
	resp, err := c.request("PUT", c.docURL(id), data)
	if err != nil {
		return "", fmt.Errorf("PUT doc failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("PUT doc %s returned %d: %s", id, resp.StatusCode, string(body))
	}

	var result struct {
		OK  bool   `json:"ok"`
		Rev string `json:"rev"`
		ID  string `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse PUT response failed: %w", err)
	}

	return result.Rev, nil
}

// PutLocalDoc creates or updates a local document.
func (c *Client) PutLocalDoc(id string, data []byte) (string, error) {
	resp, err := c.request("PUT", c.localDocURL(id), data)
	if err != nil {
		return "", fmt.Errorf("PUT local doc failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("PUT local doc %s returned %d: %s", id, resp.StatusCode, string(body))
	}

	var result struct {
		OK  bool   `json:"ok"`
		Rev string `json:"rev"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse PUT local response failed: %w", err)
	}

	return result.Rev, nil
}

// DeleteDoc deletes a document given its ID and revision.
func (c *Client) DeleteDoc(id, rev string) error {
	url := c.docURL(id) + "?rev=" + rev
	resp, err := c.request("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("DELETE doc failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE doc %s returned %d: %s", id, resp.StatusCode, string(body))
	}

	return nil
}

// EnsureDB creates the database if it doesn't exist.
func (c *Client) EnsureDB() error {
	resp, err := c.request("PUT", c.dbURL(), nil)
	if err != nil {
		return fmt.Errorf("PUT db failed: %w", err)
	}
	defer resp.Body.Close()

	// 201 Created or 412 Precondition Failed (already exists) are both OK
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusPreconditionFailed {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ensure DB returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// urlEncodeDocID properly encodes document IDs for URLs.
// Encodes each byte of the UTF-8 representation, not Unicode codepoints.
// Preserves forward slashes as path separators.
func urlEncodeDocID(id string) string {
	var encoded strings.Builder
	for _, b := range []byte(id) {
		switch {
		case b == '/':
			encoded.WriteByte('/')
		case (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
			(b >= '0' && b <= '9') || b == '-' || b == '.' || b == '_' || b == '~':
			encoded.WriteByte(b)
		default:
			encoded.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return encoded.String()
}

// GetDatabaseInfo returns basic information about the database.
func (c *Client) GetDatabaseInfo() (*DatabaseInfo, error) {
	resp, err := c.request("GET", c.dbURL(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET db info returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info DatabaseInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// DatabaseInfo holds basic database metadata.
type DatabaseInfo struct {
	DBName    string `json:"db_name"`
	DocCount  int    `json:"doc_count"`
	UpdateSeq string `json:"update_seq"`
	Sizes     struct {
		File     int64 `json:"file"`
		External int64 `json:"external"`
		Active   int64 `json:"active"`
	} `json:"sizes"`
}
