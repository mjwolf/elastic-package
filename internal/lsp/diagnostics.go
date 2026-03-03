// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lsp

import (
	"path/filepath"
	"regexp"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/elastic/elastic-package/internal/logger"
	"github.com/elastic/elastic-package/internal/packages"
	"github.com/elastic/elastic-package/internal/validation"
	"github.com/elastic/package-spec/v3/code/go/pkg/specerrors"
)

var filePathRegexp = regexp.MustCompile(`file "([^"]+)"`)

func (s *Server) publishDiagnostics(changedURI protocol.DocumentUri) {
	notify := s.getNotifyFn()
	if notify == nil {
		return
	}

	pkgRoot := s.resolvePackageRoot()
	if pkgRoot == "" {
		return
	}

	diags := s.collectDiagnostics(pkgRoot)

	// Group diagnostics by file URI
	byURI := make(map[protocol.DocumentUri][]protocol.Diagnostic)
	for _, d := range diags {
		byURI[d.uri] = append(byURI[d.uri], d.diag)
	}

	// Always send diagnostics for the changed file (clears if no issues)
	if _, ok := byURI[changedURI]; !ok {
		byURI[changedURI] = []protocol.Diagnostic{}
	}

	for uri, fileDiags := range byURI {
		notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
			URI:         uri,
			Diagnostics: fileDiags,
		})
	}
}

type fileDiagnostic struct {
	uri  protocol.DocumentUri
	diag protocol.Diagnostic
}

func (s *Server) collectDiagnostics(pkgRoot string) []fileDiagnostic {
	allErrors, _ := validation.ValidateAndFilterFromPath(pkgRoot)
	if allErrors == nil {
		return nil
	}

	var results []fileDiagnostic

	errs, ok := allErrors.(specerrors.ValidationErrors)
	if !ok {
		results = append(results, fileDiagnostic{
			uri: pathToURI(filepath.Join(pkgRoot, "manifest.yml")),
			diag: protocol.Diagnostic{
				Range:    zeroRange(),
				Severity: severityPtr(protocol.DiagnosticSeverityError),
				Source:   strPtr("elastic-package"),
				Message:  allErrors.Error(),
			},
		})
		return results
	}

	for _, ve := range errs {
		filePath := extractFilePath(ve.Error(), pkgRoot)
		msg := ve.Error()

		uri := pathToURI(filePath)
		results = append(results, fileDiagnostic{
			uri: uri,
			diag: protocol.Diagnostic{
				Range:    zeroRange(),
				Severity: severityPtr(protocol.DiagnosticSeverityError),
				Source:   strPtr("elastic-package"),
				Message:  msg,
				Code:     codeFromError(ve),
			},
		})
	}

	return results
}

func extractFilePath(errMsg, pkgRoot string) string {
	if match := filePathRegexp.FindStringSubmatch(errMsg); len(match) > 1 {
		p := match[1]
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(pkgRoot, p)
	}

	for _, prefix := range []string{"data_stream/", "kibana/", "docs/", "elasticsearch/", "agent/"} {
		if idx := strings.Index(errMsg, prefix); idx >= 0 {
			end := strings.IndexAny(errMsg[idx:], " :")
			if end == -1 {
				end = len(errMsg) - idx
			}
			candidate := errMsg[idx : idx+end]
			return filepath.Join(pkgRoot, candidate)
		}
	}

	return filepath.Join(pkgRoot, "manifest.yml")
}

func (s *Server) resolvePackageRoot() string {
	if s.packageRoot != "" {
		if _, err := packages.FindPackageRootFrom(s.packageRoot); err == nil {
			return s.packageRoot
		}
	}
	root, err := packages.FindPackageRoot()
	if err != nil {
		logger.Debugf("LSP: could not find package root: %v", err)
		return ""
	}
	return root
}

func zeroRange() protocol.Range {
	return protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: 0, Character: 0},
	}
}

func severityPtr(s protocol.DiagnosticSeverity) *protocol.DiagnosticSeverity {
	return &s
}

func codeFromError(ve specerrors.ValidationError) *protocol.IntegerOrString {
	code := ve.Code()
	if code == "" || code == specerrors.UnassignedCode {
		return nil
	}
	return &protocol.IntegerOrString{Value: code}
}
