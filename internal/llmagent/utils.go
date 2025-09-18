// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package llmagent

import "strings"

// maskAPIKey masks an API key for secure logging, showing first 8 and last 4 characters
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 12 {
		return strings.Repeat("*", len(apiKey))
	}
	return apiKey[:8] + strings.Repeat("*", len(apiKey)-12) + apiKey[len(apiKey)-4:]
}
