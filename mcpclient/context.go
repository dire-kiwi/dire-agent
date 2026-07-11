package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

type ResourceContent struct {
	URI       string `json:"uri"`
	MIMEType  string `json:"mime_type,omitempty"`
	Text      string `json:"text,omitempty"`
	BlobBytes int    `json:"blob_bytes,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type PromptInfo struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

type PromptResult struct {
	Description string `json:"description,omitempty"`
	Output      string `json:"output"`
	Truncated   bool   `json:"truncated,omitempty"`
}

type resourceSession interface {
	ListResources(context.Context, *mcp.ListResourcesParams) (*mcp.ListResourcesResult, error)
	ReadResource(context.Context, *mcp.ReadResourceParams) (*mcp.ReadResourceResult, error)
}

type promptSession interface {
	ListPrompts(context.Context, *mcp.ListPromptsParams) (*mcp.ListPromptsResult, error)
	GetPrompt(context.Context, *mcp.GetPromptParams) (*mcp.GetPromptResult, error)
}

func (c *Client) ListResources(ctx context.Context, server string) ([]ResourceInfo, error) {
	runtime, session, err := c.contextSession(server)
	if err != nil {
		return nil, err
	}
	reader, ok := session.(resourceSession)
	if !ok {
		return nil, fmt.Errorf("MCP server %q does not support resources", server)
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout(runtime.config.ListTimeout, c.options.ListTimeout))
	defer cancel()
	var resources []ResourceInfo
	cursor, seen := "", map[string]bool{}
	for page := 0; page < 1000; page++ {
		result, callErr := reader.ListResources(callCtx, &mcp.ListResourcesParams{Cursor: cursor})
		if callErr != nil {
			return nil, c.contextFailure(runtime, "listing resources", callErr)
		}
		if result == nil {
			return nil, c.contextFailure(runtime, "listing resources", errors.New("resources/list returned no result"))
		}
		for _, item := range result.Resources {
			if item == nil {
				continue
			}
			resources = append(resources, ResourceInfo{URI: item.URI, Name: item.Name, Title: item.Title, Description: item.Description, MIMEType: item.MIMEType})
			if len(resources) >= 10_000 {
				return resources, nil
			}
		}
		if result.NextCursor == "" {
			return resources, nil
		}
		if seen[result.NextCursor] {
			return nil, c.contextFailure(runtime, "listing resources", errors.New("resources/list repeated a pagination cursor"))
		}
		seen[result.NextCursor], cursor = true, result.NextCursor
	}
	return nil, c.contextFailure(runtime, "listing resources", errors.New("resources/list exceeded 1000 pages"))
}

func (c *Client) ReadResource(ctx context.Context, server, uri string) ([]ResourceContent, error) {
	if strings.TrimSpace(uri) == "" || len(uri) > 8192 {
		return nil, errors.New("MCP resource URI must be between 1 and 8192 bytes")
	}
	runtime, session, err := c.contextSession(server)
	if err != nil {
		return nil, err
	}
	reader, ok := session.(resourceSession)
	if !ok {
		return nil, fmt.Errorf("MCP server %q does not support resources", server)
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout(runtime.config.CallTimeout, c.options.CallTimeout))
	defer cancel()
	result, err := reader.ReadResource(callCtx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return nil, c.contextFailure(runtime, "reading resource", err)
	}
	if result == nil {
		return nil, c.contextFailure(runtime, "reading resource", errors.New("resources/read returned no result"))
	}
	remaining := c.options.MaxResultBytes
	contents := make([]ResourceContent, 0, len(result.Contents))
	for _, item := range result.Contents {
		if item == nil {
			continue
		}
		text, cut := "", item.Text != ""
		if remaining > 0 {
			text, cut = truncateUTF8(item.Text, remaining)
		}
		remaining -= len(text)
		contents = append(contents, ResourceContent{
			URI: item.URI, MIMEType: item.MIMEType, Text: text,
			BlobBytes: len(item.Blob), Truncated: cut || remaining <= 0 && len(item.Text) > 0,
		})
	}
	return contents, nil
}

func (c *Client) ListPrompts(ctx context.Context, server string) ([]PromptInfo, error) {
	runtime, session, err := c.contextSession(server)
	if err != nil {
		return nil, err
	}
	provider, ok := session.(promptSession)
	if !ok {
		return nil, fmt.Errorf("MCP server %q does not support prompts", server)
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout(runtime.config.ListTimeout, c.options.ListTimeout))
	defer cancel()
	var prompts []PromptInfo
	cursor, seen := "", map[string]bool{}
	for page := 0; page < 1000; page++ {
		result, callErr := provider.ListPrompts(callCtx, &mcp.ListPromptsParams{Cursor: cursor})
		if callErr != nil {
			return nil, c.contextFailure(runtime, "listing prompts", callErr)
		}
		if result == nil {
			return nil, c.contextFailure(runtime, "listing prompts", errors.New("prompts/list returned no result"))
		}
		for _, item := range result.Prompts {
			if item == nil {
				continue
			}
			prompt := PromptInfo{Name: item.Name, Title: item.Title, Description: item.Description}
			for _, argument := range item.Arguments {
				if argument != nil {
					prompt.Arguments = append(prompt.Arguments, PromptArgument{Name: argument.Name, Description: argument.Description, Required: argument.Required})
				}
			}
			prompts = append(prompts, prompt)
			if len(prompts) >= 10_000 {
				return prompts, nil
			}
		}
		if result.NextCursor == "" {
			return prompts, nil
		}
		if seen[result.NextCursor] {
			return nil, c.contextFailure(runtime, "listing prompts", errors.New("prompts/list repeated a pagination cursor"))
		}
		seen[result.NextCursor], cursor = true, result.NextCursor
	}
	return nil, c.contextFailure(runtime, "listing prompts", errors.New("prompts/list exceeded 1000 pages"))
}

func (c *Client) GetPrompt(ctx context.Context, server, name string, arguments map[string]string) (PromptResult, error) {
	if strings.TrimSpace(name) == "" || len(name) > 256 {
		return PromptResult{}, errors.New("MCP prompt name must be between 1 and 256 bytes")
	}
	runtime, session, err := c.contextSession(server)
	if err != nil {
		return PromptResult{}, err
	}
	provider, ok := session.(promptSession)
	if !ok {
		return PromptResult{}, fmt.Errorf("MCP server %q does not support prompts", server)
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout(runtime.config.CallTimeout, c.options.CallTimeout))
	defer cancel()
	result, err := provider.GetPrompt(callCtx, &mcp.GetPromptParams{Name: name, Arguments: arguments})
	if err != nil {
		return PromptResult{}, c.contextFailure(runtime, "getting prompt", err)
	}
	if result == nil {
		return PromptResult{}, c.contextFailure(runtime, "getting prompt", errors.New("prompts/get returned no result"))
	}
	parts := make([]string, 0, len(result.Messages))
	for _, message := range result.Messages {
		if message != nil {
			parts = append(parts, fmt.Sprintf("%s: %s", message.Role, flattenContent(message.Content)))
		}
	}
	output, cut := truncateUTF8(strings.Join(parts, "\n\n"), c.options.MaxResultBytes)
	return PromptResult{Description: result.Description, Output: output, Truncated: cut}, nil
}

func (c *Client) contextSession(server string) (*serverRuntime, Session, error) {
	if c.isClosed() {
		return nil, nil, ErrClosed
	}
	runtime, err := c.getServer(server)
	if err != nil {
		return nil, nil, err
	}
	runtime.mu.RLock()
	session := runtime.session
	runtime.mu.RUnlock()
	if session == nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotConnected, server)
	}
	return runtime, session, nil
}

func (c *Client) contextFailure(runtime *serverRuntime, operation string, err error) error {
	safe := safeError(runtime.config, operation, err)
	runtime.setState(StateDegraded, safe.Error())
	return safe
}

func sessionInitializeResult(session Session) *mcp.InitializeResult {
	if session == nil {
		return nil
	}
	return session.InitializeResult()
}
