package extensions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxManifestBytes = 1 << 20

type pathList []string

func (p *pathList) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		return nil
	}
	var single string
	if json.Unmarshal(data, &single) == nil {
		*p = pathList{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return fmt.Errorf("must be a path or an array of paths")
	}
	*p = many
	return nil
}

type manifestData struct {
	name        string
	version     string
	description string
	format      Format
	entrypoints []string
	skills      []string
	prompts     []string
	themes      []string
	hasMCP      bool
	hasApp      bool
}

type codexManifest struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Skills      pathList        `json:"skills"`
	Apps        json.RawMessage `json:"apps"`
	MCP         json.RawMessage `json:"mcp"`
	MCPServers  json.RawMessage `json:"mcpServers"`
}

type piManifest struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Pi          *piConfig `json:"pi"`
}

type piConfig struct {
	Extensions pathList `json:"extensions"`
	Skills     pathList `json:"skills"`
	Prompts    pathList `json:"prompts"`
	Themes     pathList `json:"themes"`
}

var errNotPiPackage = errors.New("package does not declare pi metadata")

func parseManifest(path string, format Format) (manifestData, error) {
	contents, err := readSmallFile(path)
	if err != nil {
		return manifestData{}, err
	}
	switch format {
	case FormatCodex:
		var raw codexManifest
		if err := json.Unmarshal(contents, &raw); err != nil {
			return manifestData{}, fmt.Errorf("parse Codex plugin manifest: %w", err)
		}
		if strings.TrimSpace(raw.Name) == "" {
			return manifestData{}, fmt.Errorf("Codex plugin manifest is missing name")
		}
		root := filepath.Dir(filepath.Dir(path))
		return manifestData{
			name: raw.Name, version: raw.Version, description: raw.Description,
			format: FormatCodex, skills: raw.Skills,
			hasMCP: present(raw.MCP) || present(raw.MCPServers) || fileExists(filepath.Join(root, ".mcp.json")),
			hasApp: present(raw.Apps) || fileExists(filepath.Join(root, ".app.json")),
		}, nil
	case FormatPi:
		var raw piManifest
		if err := json.Unmarshal(contents, &raw); err != nil {
			return manifestData{}, fmt.Errorf("parse Pi package manifest: %w", err)
		}
		if strings.TrimSpace(raw.Name) == "" {
			return manifestData{}, fmt.Errorf("Pi package manifest is missing name")
		}
		if raw.Pi == nil {
			return manifestData{}, errNotPiPackage
		}
		return manifestData{
			name: raw.Name, version: raw.Version, description: raw.Description,
			format: FormatPi, entrypoints: raw.Pi.Extensions, skills: raw.Pi.Skills,
			prompts: raw.Pi.Prompts, themes: raw.Pi.Themes,
		}, nil
	default:
		return manifestData{}, fmt.Errorf("unsupported manifest format %q", format)
	}
}

func readSmallFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxManifestBytes {
		return nil, fmt.Errorf("manifest exceeds %d bytes", maxManifestBytes)
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxManifestBytes {
		return nil, fmt.Errorf("manifest exceeds %d bytes", maxManifestBytes)
	}
	return contents, nil
}

func present(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte("false"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
