// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"github.com/elastic/elastic-package/internal/packages"
)

const updateDocumentationLongDescription = `Use this command to update package documentation.

The command can update the /_dev_/docs/README.md file in the package with the latest information and formatting.`

type updateDocumentationAnswers struct {
	Confirm bool
}

func updateDocumentationCommandAction(cmd *cobra.Command, args []string) error {
	cmd.Println("Update package documentation")

	packageRoot, found, err := packages.FindPackageRoot()
	if err != nil {
		return fmt.Errorf("locating package root failed: %w", err)
	}
	if !found {
		return errors.New("package root not found, you can only update documentation in the package context")
	}

	cmd.Printf("Package root found: %s\n", packageRoot)

	// Prompt user for confirmation
	qs := []*survey.Question{
		{
			Name: "confirm",
			Prompt: &survey.Confirm{
				Message: "Do you want to update the documentation",
				Default: false,
			},
			Validate: survey.Required,
		},
	}

	var answers updateDocumentationAnswers
	err = survey.Ask(qs, &answers)
	if err != nil {
		return fmt.Errorf("prompt failed: %w", err)
	}

	if !answers.Confirm {
		cmd.Println("Documentation update cancelled.")
		return nil
	}

	// TODO: Implement actual documentation update logic here
	// This would update the /_dev_/docs/README.md file in the package
	cmd.Println("Documentation update feature not yet implemented.")
	cmd.Println("In the future, this will update the /_dev_/docs/README.md file in the package.")

	cmd.Println("Done")
	return nil
}
