package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var call request
		if json.Unmarshal(scanner.Bytes(), &call) != nil {
			continue
		}
		response := map[string]any{"jsonrpc": "2.0", "id": call.ID}
		switch call.Method {
		case "initialize":
			response["result"] = map[string]any{
				"protocol_version": "1.0",
				"server":           map[string]string{"name": "browser-fixture", "version": "1"},
				"registration": map[string]any{
					"commands":         []map[string]string{{"name": "fixture", "description": "Run the browser fixture command", "argument_hint": "TEXT"}},
					"prompt_fragments": []map[string]any{{"id": "browser-fixture", "text": "Preserve exact-output requests.", "priority": 1}},
					"hooks":            []map[string]any{{"id": "observe-prompt", "event": "before_prompt", "priority": 1}},
				},
			}
		case "list_tools":
			response["result"] = map[string]any{"tools": []map[string]any{{
				"name": "echo", "description": "Echo a deterministic browser fixture value.",
				"input_schema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]string{"type": "string"}}, "required": []string{"value"}},
			}}}
		case "call_tool":
			var params struct {
				Arguments struct {
					Value string `json:"value"`
				} `json:"arguments"`
			}
			_ = json.Unmarshal(call.Params, &params)
			response["result"] = map[string]any{"output": "EXTENSION_ECHO: " + params.Arguments.Value}
		case "execute_command":
			var params struct {
				Arguments string `json:"arguments"`
			}
			_ = json.Unmarshal(call.Params, &params)
			response["result"] = map[string]any{"output": "EXTENSION_COMMAND_OK " + params.Arguments}
		case "invoke_hook":
			response["result"] = map[string]any{}
		case "get_status":
			response["result"] = map[string]any{"level": "ready", "message": "browser fixture ready"}
		case "shutdown":
			response["result"] = map[string]any{}
			_ = encoder.Encode(response)
			return
		default:
			response["error"] = map[string]any{"code": -32601, "message": fmt.Sprintf("unknown method %s", call.Method)}
		}
		_ = encoder.Encode(response)
	}
}
