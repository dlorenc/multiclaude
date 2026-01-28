package socket

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
)

// Request represents a request sent to the daemon
type Request struct {
	Command string                 `json:"command"`
	Args    map[string]interface{} `json:"args,omitempty"`
}

// Response represents a response from the daemon
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ErrorResponse creates a failure response with the given error message.
// It supports printf-style formatting.
func ErrorResponse(format string, args ...interface{}) Response {
	return Response{
		Success: false,
		Error:   fmt.Sprintf(format, args...),
	}
}

// SuccessResponse creates a successful response with optional data.
func SuccessResponse(data interface{}) Response {
	return Response{
		Success: true,
		Data:    data,
	}
}

// Client connects to the daemon via Unix socket
type Client struct {
	socketPath string
}

// NewClient creates a new socket client
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

// Send sends a request to the daemon and returns the response
func (c *Client) Send(req Request) (*Response, error) {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer conn.Close()

	// Send request
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &resp, nil
}

// Server listens on a Unix socket for requests
type Server struct {
	socketPath string
	listener   net.Listener
	handler    Handler
}

// Handler processes requests
type Handler interface {
	Handle(req Request) Response
}

// HandlerFunc is an adapter to allow functions to be used as handlers
type HandlerFunc func(Request) Response

// Handle implements the Handler interface
func (f HandlerFunc) Handle(req Request) Response {
	return f(req)
}

// NewServer creates a new socket server
func NewServer(socketPath string, handler Handler) *Server {
	return &Server{
		socketPath: socketPath,
		handler:    handler,
	}
}

// Start starts the socket server
func (s *Server) Start() error {
	// Remove stale socket file if exists
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}

	// Set permissions
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	s.listener = listener
	return nil
}

// Serve accepts and handles connections
func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept connection: %w", err)
		}

		go s.handleConnection(conn)
	}
}

// Stop stops the server
func (s *Server) Stop() error {
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			return err
		}
	}

	// Remove socket file
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// handleConnection handles a single connection
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		if err != io.EOF {
			resp := Response{
				Success: false,
				Error:   fmt.Sprintf("failed to decode request: %v", err),
			}
			json.NewEncoder(conn).Encode(resp)
		}
		return
	}

	resp := s.handler.Handle(req)
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		// Can't send error response at this point
		return
	}
}
