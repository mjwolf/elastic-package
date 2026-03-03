// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lsp

import (
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/elastic/elastic-package/internal/formatter"
	"github.com/elastic/elastic-package/internal/logger"
	"github.com/elastic/elastic-package/internal/packages"
)

func (s *Server) textDocumentFormatting(_ *glsp.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	filePath := uriToPath(params.TextDocument.URI)

	content, ok := s.getDocContent(params.TextDocument.URI)
	if !ok {
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Debugf("LSP format: cannot read file %s: %v", filePath, err)
			return nil, nil
		}
		content = string(data)
	}

	specVer := s.resolveSpecVersion()
	if specVer == nil {
		return nil, nil
	}

	formatted, alreadyFormatted, err := formatter.FormatFileContent(filePath, []byte(content), *specVer)
	if err != nil {
		logger.Debugf("LSP format error: %v", err)
		return nil, nil
	}
	if alreadyFormatted {
		return nil, nil
	}

	lines := strings.Split(content, "\n")
	return []protocol.TextEdit{
		{
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: protocol.UInteger(len(lines)), Character: 0},
			},
			NewText: string(formatted),
		},
	}, nil
}

func (s *Server) resolveSpecVersion() *semver.Version {
	pkgRoot := s.resolvePackageRoot()
	if pkgRoot == "" {
		return nil
	}
	manifest, err := packages.ReadPackageManifestFromPackageRoot(pkgRoot)
	if err != nil {
		logger.Debugf("LSP: cannot read manifest: %v", err)
		return nil
	}
	v, err := semver.NewVersion(manifest.SpecVersion)
	if err != nil {
		logger.Debugf("LSP: cannot parse spec version: %v", err)
		return nil
	}
	return v
}
