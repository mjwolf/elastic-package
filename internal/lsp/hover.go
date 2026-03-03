// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lsp

import (
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) textDocumentHover(_ *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	filePath := uriToPath(params.TextDocument.URI)
	schema := s.resolveSchemaForFile(filePath)
	if schema == nil {
		return nil, nil
	}

	content, ok := s.getDocContent(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	keyPath := resolveYAMLKeyPath(content, int(params.Position.Line), int(params.Position.Character))
	if len(keyPath) == 0 {
		return nil, nil
	}

	prop := getPropertySchema(schema, keyPath)
	if prop == nil {
		return nil, nil
	}

	doc := formatSchemaDoc(prop)
	if doc == "" {
		return nil, nil
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: doc,
		},
	}, nil
}

// resolveYAMLKeyPath extracts the YAML key path at a given line/column from raw YAML text.
// It uses indentation to determine nesting.
func resolveYAMLKeyPath(content string, line, _ int) []string {
	lines := strings.Split(content, "\n")
	if line >= len(lines) {
		return nil
	}

	currentLine := lines[line]
	trimmed := strings.TrimLeft(currentLine, " ")
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-") {
		return nil
	}

	colonIdx := strings.Index(trimmed, ":")
	if colonIdx <= 0 {
		return nil
	}
	currentKey := strings.TrimSpace(trimmed[:colonIdx])
	currentIndent := len(currentLine) - len(trimmed)

	var path []string
	for i := line - 1; i >= 0; i-- {
		l := lines[i]
		t := strings.TrimLeft(l, " ")
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "-") {
			continue
		}
		indent := len(l) - len(t)
		if indent < currentIndent {
			ci := strings.Index(t, ":")
			if ci > 0 {
				key := strings.TrimSpace(t[:ci])
				path = append([]string{key}, path...)
				currentIndent = indent
			}
		}
	}
	path = append(path, currentKey)
	return path
}
