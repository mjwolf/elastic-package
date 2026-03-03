// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lsp

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	glspServer "github.com/tliron/glsp/server"

	"github.com/elastic/elastic-package/internal/logger"
)

const serverName = "elastic-package-lsp"

// Server is the elastic-package LSP server.
type Server struct {
	handler     protocol.Handler
	glspServer  *glspServer.Server
	packageRoot string

	mu       sync.RWMutex
	docs     map[protocol.DocumentUri]string // URI -> content
	notifyFn glsp.NotifyFunc
}

// NewServer creates a new LSP server. Schema is loaded from the package-spec module's embedded FS.
func NewServer() *Server {
	s := &Server{
		docs: make(map[protocol.DocumentUri]string),
	}

	s.handler.Initialize = s.initialize
	s.handler.Initialized = s.initialized
	s.handler.Shutdown = s.shutdown
	s.handler.Exit = s.exit
	s.handler.TextDocumentDidOpen = s.textDocumentDidOpen
	s.handler.TextDocumentDidChange = s.textDocumentDidChange
	s.handler.TextDocumentDidSave = s.textDocumentDidSave
	s.handler.TextDocumentDidClose = s.textDocumentDidClose
	s.handler.TextDocumentFormatting = s.textDocumentFormatting
	s.handler.TextDocumentHover = s.textDocumentHover
	s.handler.TextDocumentCompletion = s.textDocumentCompletion

	s.glspServer = glspServer.NewServer(&s.handler, serverName, false)

	return s
}

// RunStdio runs the LSP server over stdin/stdout.
func (s *Server) RunStdio() error {
	return s.glspServer.RunStdio()
}

func (s *Server) initialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	s.mu.Lock()
	s.notifyFn = ctx.Notify
	s.mu.Unlock()

	if params.RootURI != nil {
		root := uriToPath(*params.RootURI)
		s.packageRoot = root
		logger.Debugf("LSP initialized with root: %s", root)
	} else if params.RootPath != nil {
		s.packageRoot = *params.RootPath
	}

	capabilities := s.handler.CreateServerCapabilities()
	syncKind := protocol.TextDocumentSyncKindFull
	capabilities.TextDocumentSync = &protocol.TextDocumentSyncOptions{
		OpenClose: boolPtr(true),
		Change:    &syncKind,
		Save:      &protocol.True,
	}

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    serverName,
			Version: strPtr("0.1.0"),
		},
	}, nil
}

func (s *Server) initialized(_ *glsp.Context, _ *protocol.InitializedParams) error {
	logger.Debugf("LSP server initialized")
	return nil
}

func (s *Server) shutdown(_ *glsp.Context) error {
	return nil
}

func (s *Server) exit(_ *glsp.Context) error {
	os.Exit(0)
	return nil
}

func (s *Server) textDocumentDidOpen(_ *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.mu.Lock()
	s.docs[params.TextDocument.URI] = params.TextDocument.Text
	s.mu.Unlock()
	s.publishDiagnostics(params.TextDocument.URI)
	return nil
}

func (s *Server) textDocumentDidChange(_ *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	s.mu.Lock()
	for _, change := range params.ContentChanges {
		if c, ok := change.(protocol.TextDocumentContentChangeEventWhole); ok {
			s.docs[params.TextDocument.URI] = c.Text
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Server) textDocumentDidSave(_ *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	s.publishDiagnostics(params.TextDocument.URI)
	return nil
}

func (s *Server) textDocumentDidClose(_ *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.mu.Lock()
	delete(s.docs, params.TextDocument.URI)
	s.mu.Unlock()
	return nil
}

func (s *Server) getDocContent(uri protocol.DocumentUri) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.docs[uri]
	return c, ok
}

func (s *Server) getNotifyFn() glsp.NotifyFunc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.notifyFn
}

func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return strings.TrimPrefix(uri, "file://")
	}
	return u.Path
}

func pathToURI(path string) protocol.DocumentUri {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return fmt.Sprintf("file://%s", abs)
}

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }
