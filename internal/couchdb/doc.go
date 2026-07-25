package couchdb

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// GetDocJSON retrieves a document and unmarshals it into the target value.
func (c *Client) GetDocJSON(id string, target interface{}) error {
	data, err := c.GetDoc(id)
	if err != nil {
		return err
	}
	if data == nil {
		return nil // Not found
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal doc %s failed: %w", id, err)
	}
	return nil
}

// GetLocalDocJSON retrieves a local document and unmarshals it.
func (c *Client) GetLocalDocJSON(id string, target interface{}) error {
	data, err := c.GetLocalDoc(id)
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal local doc %s failed: %w", id, err)
	}
	return nil
}

// PutDocJSON marshals and puts a document.
func (c *Client) PutDocJSON(id string, doc interface{}) (string, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal doc failed: %w", err)
	}
	return c.PutDoc(id, data)
}

// PutLocalDocJSON marshals and puts a local document.
func (c *Client) PutLocalDocJSON(id string, doc interface{}) (string, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal local doc failed: %w", err)
	}
	return c.PutLocalDoc(id, data)
}

// HasDatabase checks if the database exists.
func (c *Client) HasDatabase() (bool, error) {
	resp, err := c.request("GET", c.dbURL(), nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}
