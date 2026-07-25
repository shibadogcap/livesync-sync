// Package api provides MCP (Model Context Protocol) server support.
// Implements Streamable HTTP transport for MCP over JSON-RPC 2.0.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// MCPHandler implements the Model Context Protocol over Streamable HTTP.
type MCPHandler struct {
	ops *VaultOps

	// Tool definitions
	tools []ToolDef
}

// ToolDef defines an MCP tool.
type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema Schema `json:"inputSchema"`
}

// Schema is a simplified JSON Schema.
type Schema struct {
	Type       string            `json:"type"`
	Properties map[string]Prop   `json:"properties,omitempty"`
	Required   []string          `json:"required,omitempty"`
}

// Prop is a JSON Schema property.
type Prop struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"-"`
}

// NewMCPHandler creates a new MCP handler.
func NewMCPHandler(ops *VaultOps) *MCPHandler {
	h := &MCPHandler{ops: ops}
	h.registerTools()
	return h
}

func (h *MCPHandler) registerTools() {
	h.tools = []ToolDef{
		{
			Name:        "vault_list",
			Description: "List files and directories in the vault at the specified path",
			InputSchema: Schema{
				Type: "object",
				Properties: map[string]Prop{
					"path": {Type: "string", Description: "Vault path to list (default: root)"},
				},
			},
		},
		{
			Name:        "vault_read",
			Description: "Read the content of a file in the vault",
			InputSchema: Schema{
				Type: "object",
				Properties: map[string]Prop{
					"path": {Type: "string", Description: "Path to the file", Required: true},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "vault_write",
			Description: "Create or overwrite a file in the vault",
			InputSchema: Schema{
				Type: "object",
				Properties: map[string]Prop{
					"path":    {Type: "string", Description: "Path to the file", Required: true},
					"content": {Type: "string", Description: "File content", Required: true},
				},
				Required: []string{"path", "content"},
			},
		},
		{
			Name:        "vault_append",
			Description: "Append content to an existing file in the vault",
			InputSchema: Schema{
				Type: "object",
				Properties: map[string]Prop{
					"path":    {Type: "string", Description: "Path to the file", Required: true},
					"content": {Type: "string", Description: "Content to append", Required: true},
				},
				Required: []string{"path", "content"},
			},
		},
		{
			Name:        "vault_delete",
			Description: "Delete a file in the vault (moves to .trash unless permanent=true)",
			InputSchema: Schema{
				Type: "object",
				Properties: map[string]Prop{
					"path":      {Type: "string", Description: "Path to the file", Required: true},
					"permanent": {Type: "boolean", Description: "If true, permanently delete"},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "vault_search",
			Description: "Search for files in the vault by name or path",
			InputSchema: Schema{
				Type: "object",
				Properties: map[string]Prop{
					"query": {Type: "string", Description: "Search query", Required: true},
				},
				Required: []string{"query"},
			},
		},
	}
}

// HandleRequest handles an MCP request (POST /mcp/).
func (h *MCPHandler) HandleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSONRPCError(w, nil, -32700, "Parse error: invalid JSON")
		return
	}

	h.handleMessage(w, r, &request)
}

func (h *MCPHandler) handleMessage(w http.ResponseWriter, r *http.Request, req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		h.handleInitialize(w, req)
	case "ping":
		h.handlePing(w, req)
	case "tools/list":
		h.handleToolsList(w, req)
	case "tools/call":
		h.handleToolCall(w, req)
	case "resources/list":
		h.handleResourcesList(w, req)
	case "notifications/initialized":
		// No response needed for notifications
		w.WriteHeader(http.StatusAccepted)
	default:
		writeJSONRPCError(w, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func (h *MCPHandler) handleInitialize(w http.ResponseWriter, req *JSONRPCRequest) {
	writeJSONRPC(w, req.ID, map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{},
		},
		"serverInfo": map[string]string{
			"name":    "livesync-sync",
			"version": "0.1.0",
		},
	})
}

func (h *MCPHandler) handlePing(w http.ResponseWriter, req *JSONRPCRequest) {
	writeJSONRPC(w, req.ID, map[string]interface{}{})
}

func (h *MCPHandler) handleToolsList(w http.ResponseWriter, req *JSONRPCRequest) {
	writeJSONRPC(w, req.ID, map[string]interface{}{
		"tools": h.tools,
	})
}

func (h *MCPHandler) handleToolCall(w http.ResponseWriter, req *JSONRPCRequest) {
	params, ok := req.Params.(map[string]interface{})
	if !ok {
		writeJSONRPCError(w, req.ID, -32602, "Invalid params")
		return
	}

	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]interface{})

	var result *ToolResult
	switch name {
	case "vault_list":
		result = h.callVaultList(args)
	case "vault_read":
		result = h.callVaultRead(args)
	case "vault_write":
		result = h.callVaultWrite(args)
	case "vault_append":
		result = h.callVaultAppend(args)
	case "vault_delete":
		result = h.callVaultDelete(args)
	case "vault_search":
		result = h.callVaultSearch(args)
	default:
		writeJSONRPCError(w, req.ID, -32601, fmt.Sprintf("Unknown tool: %s", name))
		return
	}

	writeJSONRPC(w, req.ID, map[string]interface{}{
		"content": result.Content,
		"isError": result.IsError,
	})
}

func (h *MCPHandler) handleResourcesList(w http.ResponseWriter, req *JSONRPCRequest) {
	writeJSONRPC(w, req.ID, map[string]interface{}{
		"resources": []interface{}{},
	})
}

// ToolResult is the result of a tool call.
type ToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem is a piece of MCP content.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textContent(text string) *ToolResult {
	return &ToolResult{
		Content: []ContentItem{{Type: "text", Text: text}},
	}
}

func errorContent(text string) *ToolResult {
	return &ToolResult{
		Content: []ContentItem{{Type: "text", Text: text}},
		IsError: true,
	}
}

func (h *MCPHandler) callVaultList(args map[string]interface{}) *ToolResult {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	entry, err := h.ops.ListDir(path)
	if err != nil {
		return errorContent(fmt.Sprintf("Failed to list: %v", err))
	}

	data, _ := json.MarshalIndent(entry, "", "  ")
	return textContent(string(data))
}

func (h *MCPHandler) callVaultRead(args map[string]interface{}) *ToolResult {
	path, _ := args["path"].(string)
	if path == "" {
		return errorContent("path is required")
	}

	content, info, err := h.ops.ReadFile(path)
	if err != nil {
		return errorContent(fmt.Sprintf("Failed to read: %v", err))
	}

	meta, _ := json.Marshal(info)
	return textContent(fmt.Sprintf("--- File: %s\n%s\n\n--- Content:\n%s", info.Rel, string(meta), string(content)))
}

func (h *MCPHandler) callVaultWrite(args map[string]interface{}) *ToolResult {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	if path == "" {
		return errorContent("path is required")
	}

	if err := h.ops.WriteFile(path, []byte(content)); err != nil {
		return errorContent(fmt.Sprintf("Failed to write: %v", err))
	}

	return textContent(fmt.Sprintf("Written %d bytes to %s", len(content), path))
}

func (h *MCPHandler) callVaultAppend(args map[string]interface{}) *ToolResult {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	if path == "" {
		return errorContent("path is required")
	}

	if err := h.ops.AppendFile(path, []byte(content)); err != nil {
		return errorContent(fmt.Sprintf("Failed to append: %v", err))
	}

	return textContent(fmt.Sprintf("Appended %d bytes to %s", len(content), path))
}

func (h *MCPHandler) callVaultDelete(args map[string]interface{}) *ToolResult {
	path, _ := args["path"].(string)
	permanent, _ := args["permanent"].(bool)

	if path == "" {
		return errorContent("path is required")
	}

	if err := h.ops.DeleteFile(path, permanent); err != nil {
		return errorContent(fmt.Sprintf("Failed to delete: %v", err))
	}

	mode := "trashed"
	if permanent {
		mode = "permanently deleted"
	}
	return textContent(fmt.Sprintf("%s %s", path, mode))
}

func (h *MCPHandler) callVaultSearch(args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)

	results, err := h.ops.SearchFiles(query)
	if err != nil {
		return errorContent(fmt.Sprintf("Search failed: %v", err))
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return textContent(string(data))
}

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	ID      interface{} `json:"id,omitempty"`
	Params  interface{} `json:"params,omitempty"`
}

// writeJSONRPC writes a JSON-RPC 2.0 success response.
func writeJSONRPC(w http.ResponseWriter, id interface{}, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

// writeJSONRPCError writes a JSON-RPC 2.0 error response.
func writeJSONRPCError(w http.ResponseWriter, id interface{}, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // MCP uses 200 even for errors
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
}
