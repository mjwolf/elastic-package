// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/elastic/elastic-package/internal/cobraext"
)

const updateLongDescription = `Use this command to update package resources.

The command can help update various aspects of a package such as documentation, configuration files, and other resources.`

func setupUpdateCommand() *cobraext.Command {
	updateDocumentationCmd := &cobra.Command{
		Use:   "documentation",
		Short: "Update package documentation",
		Long:  updateDocumentationLongDescription,
		Args:  cobra.NoArgs,
		RunE:  updateDocumentationCommandAction,
	}

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update package resources",
		Long:  updateLongDescription,
	}
	cmd.AddCommand(updateDocumentationCmd)

	return cobraext.NewCommand(cmd, cobraext.ContextGlobal)
}
