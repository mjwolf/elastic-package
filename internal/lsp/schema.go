// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lsp

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	spec "github.com/elastic/package-spec/v3"
	"github.com/elastic/elastic-package/internal/logger"
	"gopkg.in/yaml.v3"
)

// specSchema is the top-level wrapper for a package-spec YAML schema file.
type specSchema struct {
	Spec jsonSchema `yaml:"spec"`
}

// jsonSchema represents a simplified JSON Schema as used in package-spec YAML files.
type jsonSchema struct {
	Type                 string                `yaml:"type"`
	Description          string                `yaml:"description"`
	Properties           map[string]jsonSchema `yaml:"properties"`
	Items                *jsonSchema           `yaml:"items"`
	Enum                 []any                 `yaml:"enum"`
	Required             []string              `yaml:"required"`
	AdditionalProperties *bool                 `yaml:"additionalProperties"`
	Examples             []any                 `yaml:"examples"`
	Pattern              string                `yaml:"pattern"`
	Default              any                   `yaml:"default"`
	Definitions          map[string]jsonSchema `yaml:"definitions"`
	Ref                  string                `yaml:"$ref"`
}

// loadSpecSchemaFromFS loads and parses a package-spec schema file from an fs.FS.
func loadSpecSchemaFromFS(fsys fs.FS, path string) (*jsonSchema, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	var s specSchema
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing spec schema %s: %w", path, err)
	}
	return &s.Spec, nil
}

// resolveSchemaForFile determines which spec schema applies to a given file within a package.
func (s *Server) resolveSchemaForFile(filePath string) *jsonSchema {
	pkgRoot := s.resolvePackageRoot()
	if pkgRoot == "" {
		return nil
	}

	rel, err := filepath.Rel(pkgRoot, filePath)
	if err != nil {
		return nil
	}
	rel = filepath.ToSlash(rel)

	specFS := spec.FS()
	specBase := "integration"

	var schemaPath string
	switch {
	case rel == "manifest.yml":
		schemaPath = specBase + "/manifest.spec.yml"
	case rel == "changelog.yml":
		schemaPath = specBase + "/changelog.spec.yml"
	case strings.HasPrefix(rel, "data_stream/"):
		parts := strings.Split(rel, "/")
		if len(parts) >= 3 && parts[2] == "manifest.yml" {
			schemaPath = specBase + "/data_stream/manifest.spec.yml"
		} else if len(parts) >= 3 && parts[2] == "fields" {
			schemaPath = specBase + "/data_stream/fields/fields.spec.yml"
		}
	}

	if schemaPath == "" {
		return nil
	}

	schema, err := loadSpecSchemaFromFS(specFS, schemaPath)
	if err != nil {
		logger.Debugf("LSP: cannot load schema %s: %v", schemaPath, err)
		return nil
	}

	resolveRefs(schema, specFS, specBase)

	return schema
}

// resolveRefs resolves $ref references by loading the referenced schema file from the embedded FS.
func resolveRefs(schema *jsonSchema, fsys fs.FS, specBase string) {
	if schema == nil {
		return
	}
	for k, prop := range schema.Properties {
		if prop.Ref != "" {
			refPath := specBase + "/" + strings.TrimPrefix(prop.Ref, "./")
			if refSchema, err := loadSpecSchemaFromFS(fsys, refPath); err == nil {
				schema.Properties[k] = *refSchema
			}
		} else {
			p := prop
			resolveRefs(&p, fsys, specBase)
			schema.Properties[k] = p
		}
	}
	if schema.Items != nil && schema.Items.Ref != "" {
		refPath := specBase + "/" + strings.TrimPrefix(schema.Items.Ref, "./")
		if refSchema, err := loadSpecSchemaFromFS(fsys, refPath); err == nil {
			schema.Items = refSchema
		}
	}
}

// getPropertySchema returns the schema for a given YAML key path (e.g. "owner.type").
func getPropertySchema(schema *jsonSchema, keyPath []string) *jsonSchema {
	if schema == nil || len(keyPath) == 0 {
		return schema
	}
	key := keyPath[0]

	if schema.Definitions != nil {
		if def, ok := schema.Definitions[key]; ok && len(keyPath) == 1 {
			return &def
		}
	}

	if schema.Properties != nil {
		if prop, ok := schema.Properties[key]; ok {
			if len(keyPath) == 1 {
				return &prop
			}
			return getPropertySchema(&prop, keyPath[1:])
		}
	}
	return nil
}

// schemaPropertyNames returns the top-level property names for a given schema.
func schemaPropertyNames(schema *jsonSchema) []string {
	if schema == nil || schema.Properties == nil {
		return nil
	}
	names := make([]string, 0, len(schema.Properties))
	for k := range schema.Properties {
		names = append(names, k)
	}
	return names
}

// formatSchemaDoc formats a schema property as markdown for hover display.
func formatSchemaDoc(prop *jsonSchema) string {
	var b strings.Builder
	if prop.Description != "" {
		b.WriteString(prop.Description)
		b.WriteString("\n\n")
	}
	if prop.Type != "" {
		fmt.Fprintf(&b, "**Type:** `%s`\n\n", prop.Type)
	}
	if len(prop.Enum) > 0 {
		b.WriteString("**Allowed values:** ")
		for i, v := range prop.Enum {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "`%v`", v)
		}
		b.WriteString("\n\n")
	}
	if prop.Pattern != "" {
		fmt.Fprintf(&b, "**Pattern:** `%s`\n\n", prop.Pattern)
	}
	if len(prop.Examples) > 0 {
		b.WriteString("**Examples:** ")
		for i, v := range prop.Examples {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "`%v`", v)
		}
		b.WriteString("\n")
	}
	if prop.Default != nil {
		fmt.Fprintf(&b, "**Default:** `%v`\n", prop.Default)
	}
	return strings.TrimSpace(b.String())
}
