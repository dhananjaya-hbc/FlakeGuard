package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
)

const protocolVersion = "2024-11-05"

type ToolHandler func(ctx context.Context, arguments json.RawMessage) (*ToolCallResult, error)

type Server struct {
	name     string
	version  string
	tools    []Tool
	handlers map[string]ToolHandler
}

func NewServer(name, version string) *Server {
	return &Server{
		name:     name,
		version:  version,
		handlers: make(map[string]ToolHandler),
	}
}

func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.tools = append(s.tools, tool)
	s.handlers[tool.Name] = handler
}

func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		s.handle(ctx, req, out)
	}

	return scanner.Err()
}

func (s *Server) handle(ctx context.Context, req Request, out io.Writer) {
	switch req.Method {
	case "initialize":
		s.respond(out, req.ID, InitializeResult{
			ProtocolVersion: protocolVersion,
			Capabilities:    Capabilities{Tools: map[string]interface{}{}},
			ServerInfo:      ServerInfo{Name: s.name, Version: s.version},
		}, nil)

	case "notifications/initialized":
		// notification, no response expected

	case "tools/list":
		s.respond(out, req.ID, ToolsListResult{Tools: s.tools}, nil)

	case "tools/call":
		s.handleToolCall(ctx, req, out)

	default:
		s.respond(out, req.ID, nil, &Error{Code: -32601, Message: "method not found: " + req.Method})
	}
}

func (s *Server) handleToolCall(ctx context.Context, req Request, out io.Writer) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.respond(out, req.ID, nil, &Error{Code: -32602, Message: "invalid params"})
		return
	}

	handler, ok := s.handlers[params.Name]
	if !ok {
		s.respond(out, req.ID, nil, &Error{Code: -32602, Message: "unknown tool: " + params.Name})
		return
	}

	result, err := handler(ctx, params.Arguments)
	if err != nil {
		s.respond(out, req.ID, ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		}, nil)
		return
	}

	s.respond(out, req.ID, result, nil)
}

func (s *Server) respond(out io.Writer, id json.RawMessage, result interface{}, errObj *Error) {
	if len(id) == 0 {
		return
	}

	resp := Response{JSONRPC: "2.0", ID: id}
	if errObj != nil {
		resp.Error = errObj
	} else {
		resp.Result = result
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	out.Write(data)
	out.Write([]byte("\n"))
}
