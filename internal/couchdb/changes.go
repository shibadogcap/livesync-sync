package couchdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Change represents a single change from the CouchDB changes feed.
type Change struct {
	Seq      string          `json:"seq"`
	ID       string          `json:"id"`
	Changes  []ChangeRev     `json:"changes"`
	Deleted  bool            `json:"deleted,omitempty"`
	Doc      json.RawMessage `json:"doc,omitempty"`
}

// ChangeRev represents a revision in a change entry.
type ChangeRev struct {
	Rev string `json:"rev"`
}

// ChangesResponse is the response from the _changes endpoint.
type ChangesResponse struct {
	Results  []Change `json:"results"`
	LastSeq  string   `json:"last_seq"`
	Pending  int      `json:"pending"`
}

// ChangesFeedOptions configures the changes feed request.
type ChangesFeedOptions struct {
	Since      string // Start sequence (empty = "now")
	Limit      int    // Max results (0 = unlimited)
	Timeout    int    // Long-poll timeout in ms (default 60000)
	IncludeDocs bool  // Include document content
	Feed       string // "normal", "longpoll", "continuous"
}

// DefaultChangesOptions returns sensible defaults for changes feed.
func DefaultChangesOptions() ChangesFeedOptions {
	return ChangesFeedOptions{
		Since:       "now",
		Limit:       0,
		Timeout:     60000,
		IncludeDocs: true,
		Feed:        "longpoll",
	}
}

// FetchChanges retrieves changes from the database.
// This is the main method for the changes feed (long-poll).
func (c *Client) FetchChanges(opts ChangesFeedOptions) (*ChangesResponse, error) {
	// Build query parameters
	params := []string{}
	if opts.Since != "" {
		params = append(params, "since="+opts.Since)
	}
	if opts.Limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", opts.Limit))
	}
	if opts.Timeout > 0 {
		params = append(params, fmt.Sprintf("timeout=%d", opts.Timeout))
	}
	if opts.IncludeDocs {
		params = append(params, "include_docs=true")
	}
	if opts.Feed != "" {
		params = append(params, "feed="+opts.Feed)
	}

	url := c.dbURL() + "/_changes?" + strings.Join(params, "&")

	// Create a custom request with a longer timeout for long-poll
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.setAuth(req)
	req.Header.Set("Accept", "application/json")

	// Use a dedicated long-poll client (reuse to avoid connection leak)
	timeout := time.Duration(opts.Timeout+15000) * time.Millisecond
	if timeout < 75000 {
		timeout = 75000
	}

	resp, err := c.getChangesClient(timeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("changes feed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("changes feed returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read changes response failed: %w", err)
	}

	var result ChangesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse changes response failed: %w", err)
	}

	return &result, nil
}

// BulkGetDocs retrieves multiple documents by their IDs.
// Uses the _all_docs endpoint with include_docs=true.
func (c *Client) BulkGetDocs(ids []string) (map[string][]byte, error) {
	if len(ids) == 0 {
		return make(map[string][]byte), nil
	}

	// Build _all_docs request
	body := struct {
		Keys        []string `json:"keys"`
		IncludeDocs bool     `json:"include_docs"`
	}{
		Keys:        ids,
		IncludeDocs: true,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := c.dbURL() + "/_all_docs?include_docs=true"
	resp, err := c.request("POST", url, payload)
	if err != nil {
		return nil, fmt.Errorf("bulk get failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bulk get returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Rows []struct {
			ID   string          `json:"id"`
			Doc  json.RawMessage `json:"doc"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	docs := make(map[string][]byte, len(result.Rows))
	for _, row := range result.Rows {
		if row.Doc != nil && string(row.Doc) != "null" {
			docs[row.ID] = []byte(row.Doc)
		}
	}

	return docs, nil
}
