// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lsp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) textDocumentCompletion(_ *glsp.Context, params *protocol.CompletionParams) (any, error) {
	filePath := uriToPath(params.TextDocument.URI)
	schema := s.resolveSchemaForFile(filePath)
	if schema == nil {
		return nil, nil
	}

	content, ok := s.getDocContent(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	parentPath := resolveParentKeyPath(content, int(params.Position.Line), int(params.Position.Character))

	var targetSchema *jsonSchema
	if len(parentPath) == 0 {
		targetSchema = schema
	} else {
		targetSchema = getPropertySchema(schema, parentPath)
	}
	if targetSchema == nil {
		return nil, nil
	}

	names := schemaPropertyNames(targetSchema)
	sort.Strings(names)

	var items []protocol.CompletionItem
	for _, name := range names {
		prop := targetSchema.Properties[name]
		item := protocol.CompletionItem{
			Label:  name,
			Kind:   completionKindPtr(protocol.CompletionItemKindProperty),
			Detail: completionDetail(&prop),
		}
		if prop.Description != "" {
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.MarkupKindMarkdown,
				Value: prop.Description,
			}
		}
		items = append(items, item)
	}

	if len(targetSchema.Enum) > 0 {
		for _, v := range targetSchema.Enum {
			items = append(items, protocol.CompletionItem{
				Label: fmt.Sprintf("%v", v),
				Kind:  completionKindPtr(protocol.CompletionItemKindValue),
			})
		}
	}

	return items, nil
}

// resolveParentKeyPath finds the YAML key path of the parent context at the cursor position.
func resolveParentKeyPath(content string, line, _ int) []string {
	lines := strings.Split(content, "\n")
	if line >= len(lines) {
		return nil
	}

	currentLine := lines[line]
	currentIndent := len(currentLine) - len(strings.TrimLeft(currentLine, " "))

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
	return path
}

func completionDetail(prop *jsonSchema) *string {
	if prop.Type != "" {
		return &prop.Type
	}
	return nil
}

func completionKindPtr(k protocol.CompletionItemKind) *protocol.CompletionItemKind {
	return &k
}
