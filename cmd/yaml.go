package main

import (
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

var (
	chartsToUpdateInNewRelease = map[string]map[string]string{
		"rke2-cilium": {"repo": "rancher-rke2-charts", "version": "1.19.000"},
		"rke2-canal":  {"repo": "rancher-rke2-charts", "version": "v3.31.0"},
	}
)

func update(versions ...string) error {
	if len(versions) == 0 {
		return fmt.Errorf("versions list not provided")
	}
	return nil
}

// Helper to create a scalar YAML node (for keys or simple string values)
func createScalarNode(value string) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str", // Explicitly a string
		Value: value,
	}
}

// Helper to create a mapping node for a chart entry {repo: ..., version: ...}
func createChartEntryNode(repo, version string) *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			createScalarNode("repo"), createScalarNode(repo),
			createScalarNode("version"), createScalarNode(version),
		},
	}
}

// strictlyAlphanumeric sanitizes a string to be purely alphanumeric.
func strictlyAlphanumeric(input string) string {
	var sb strings.Builder
	for _, r := range input {
		if ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
