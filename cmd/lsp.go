// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/elastic/elastic-package/internal/cobraext"
	"github.com/elastic/elastic-package/internal/lsp"
)

const lspLongDescription = `Use this command to start the Language Server Protocol (LSP) server.

The LSP server runs as a non-interactive process over stdio, providing
schema-aware diagnostics, formatting, completion, and hover for Elastic
package files. It uses the package-spec schema from the package-spec module.

Editors like VS Code, Cursor, and others can connect to this server
using their LSP client configuration.`

func setupLSPCommand() *cobraext.Command {
	cmd := &cobra.Command{
		Use:   "lsp",
		Short: "Start the LSP server for Elastic packages",
		Long:  lspLongDescription,
		Args:  cobra.NoArgs,
		RunE:  lspCommandAction,
	}

	return cobraext.NewCommand(cmd, cobraext.ContextGlobal)
}

func lspCommandAction(_ *cobra.Command, _ []string) error {
	server := lsp.NewServer()
	return server.RunStdio()
}
