package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"aibuddy/backend/internal/domain"
)

// Минимальная реализация протокола MCP (JSON-RPC 2.0) поверх шлюза:
// initialize, ping, tools/list, tools/call. Этого достаточно, чтобы подключить
// реестр к корпоративной платформе ИИ-агентов или любому MCP-клиенту (POST /mcp).

// ProtocolVersion — версия протокола MCP, объявляемая сервером.
const ProtocolVersion = "2025-06-18"

// Коды ошибок JSON-RPC.
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
)

// RPCRequest — запрос JSON-RPC 2.0.
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError — ошибка JSON-RPC 2.0.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RPCResponse — ответ JSON-RPC 2.0.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

func rpcFail(id json.RawMessage, code int, msg string) *RPCResponse {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &RPCResponse{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: msg}}
}

// HandleRPC обрабатывает один JSON-RPC запрос. Для уведомлений (без id) возвращает nil.
func (g *Gateway) HandleRPC(ctx context.Context, p domain.Principal, internID string, raw []byte) *RPCResponse {
	var req RPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return rpcFail(nil, rpcParseError, "некорректный JSON")
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		return rpcFail(req.ID, rpcInvalidRequest, "ожидается JSON-RPC 2.0 с полем method")
	}
	if len(req.ID) == 0 {
		return nil // уведомление (например notifications/initialized) — ответа нет
	}
	ok := func(result any) *RPCResponse { return &RPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result} }

	switch req.Method {
	case "initialize":
		return ok(map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "avito-start-ai-buddy-mcp", "version": "1.0.0"},
			"instructions":    "Имитации корпоративных сервисов Авито. Все данные синтетические. Каждый вызов проходит RBAC.",
		})
	case "ping":
		return ok(map[string]any{})
	case "tools/list":
		tools := make([]map[string]any, 0, len(g.reg.order))
		for _, t := range g.reg.Tools() {
			item := map[string]any{"name": t.Name, "title": t.Title, "description": t.Description, "inputSchema": t.InputSchema}
			if t.OutputSchema != nil {
				item["outputSchema"] = t.OutputSchema
			}
			tools = append(tools, item)
		}
		return ok(map[string]any{"tools": tools})
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
			return rpcFail(req.ID, rpcInvalidParams, "ожидаются params.name и params.arguments")
		}
		call := g.Execute(ctx, p, internID, params.Name, params.Arguments)
		if call.ErrorCode == CodeUnknownTool {
			return rpcFail(req.ID, rpcInvalidParams, call.Error)
		}
		if !call.OK() {
			text := fmt.Sprintf("%s: %s", call.ErrorCode, call.Error)
			return ok(map[string]any{
				"isError": true,
				"content": []map[string]any{{"type": "text", "text": text}},
				"_meta":   map[string]any{"rbac": call.RBAC, "tool_call_id": call.ID},
			})
		}
		text, _ := json.Marshal(call.Result)
		return ok(map[string]any{
			"isError":           false,
			"content":           []map[string]any{{"type": "text", "text": string(text)}},
			"structuredContent": call.Result,
			"_meta":             map[string]any{"rbac": call.RBAC, "tool_call_id": call.ID},
		})
	default:
		return rpcFail(req.ID, rpcMethodNotFound, fmt.Sprintf("метод %q не поддерживается", req.Method))
	}
}
