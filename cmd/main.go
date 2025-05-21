package main

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap" // Uber Zap for logging
	"gopkg.in/yaml.v3"
)

// Global logger (or you can pass it around)
var logger *zap.Logger

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

func main() {
	// Initialize Zap logger
	var initErr error
	logger, initErr = zap.NewDevelopment() // Or zap.NewProduction()
	if initErr != nil {
		fmt.Printf("Failed to initialize zap logger: %v\n", initErr)
		os.Exit(1)
	}
	defer logger.Sync() // Flushes buffer, if any

	inputFile := "channels-rke2.yaml"
	outputFile := "channels-rke2.output.yaml"

	// --- 1. Read and Parse the YAML file ---
	yamlBytes, err := os.ReadFile(inputFile)
	if err != nil {
		logger.Fatal("Failed to read YAML file", zap.String("file", inputFile), zap.Error(err))
	}

	var rootNode yaml.Node
	err = yaml.Unmarshal(yamlBytes, &rootNode)
	if err != nil {
		logger.Fatal("Failed to unmarshal YAML", zap.Error(err))
	}

	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		logger.Fatal("Expected a YAML document node at the root.")
	}
	docContent := rootNode.Content[0]

	// --- 2. Navigate to the 'releases' sequence ---
	var releasesSeqNode *yaml.Node
	if docContent.Kind == yaml.MappingNode {
		for i := 0; i < len(docContent.Content); i += 2 {
			keyNode := docContent.Content[i]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "releases" {
				releasesSeqNode = docContent.Content[i+1]
				break
			}
		}
	}
	if releasesSeqNode == nil || releasesSeqNode.Kind != yaml.SequenceNode {
		logger.Fatal("Could not find 'releases' sequence in YAML or it's not a sequence.")
	}
	if len(releasesSeqNode.Content) == 0 {
		logger.Fatal("'releases' sequence is empty. Cannot determine the last release.")
	}

	// --- 3. Extract Information from the Last Existing Release ---
	lastReleaseMapNode := releasesSeqNode.Content[len(releasesSeqNode.Content)-1]
	if lastReleaseMapNode.Kind != yaml.MappingNode {
		logger.Fatal("Last item in 'releases' sequence is not a map.")
	}

	var prevMinChannelServerVersion, prevMaxChannelServerVersion, baseChartsAnchorName string
	var lastReleaseVersionString string

	for i := 0; i < len(lastReleaseMapNode.Content); i += 2 {
		keyNode := lastReleaseMapNode.Content[i]
		valueNode := lastReleaseMapNode.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode {
			switch keyNode.Value {
			case "version":
				if valueNode.Kind == yaml.ScalarNode {
					lastReleaseVersionString = valueNode.Value
				}
			case "minChannelServerVersion":
				if valueNode.Kind == yaml.ScalarNode {
					prevMinChannelServerVersion = valueNode.Value
				}
			case "maxChannelServerVersion":
				if valueNode.Kind == yaml.ScalarNode {
					prevMaxChannelServerVersion = valueNode.Value
				}
			case "charts":
				if valueNode.Kind == yaml.MappingNode && valueNode.Anchor != "" {
					baseChartsAnchorName = valueNode.Anchor // This anchor name is from the file, assume it's valid
					logger.Debug("Found charts anchor from last release", zap.String("anchorName", baseChartsAnchorName))
				} else {
					logger.Warn("'charts' in last release does not have an anchor or is not a map.",
						zap.String("releaseVersion", lastReleaseVersionString),
						zap.Any("nodeKind", valueNode.Kind),
						zap.String("anchorValue", valueNode.Anchor))
				}
			}
		}
	}

	if prevMinChannelServerVersion == "" || prevMaxChannelServerVersion == "" || baseChartsAnchorName == "" {
		logger.Fatal("Could not extract all required fields from last release.",
			zap.String("lastReleaseVersion", lastReleaseVersionString),
			zap.String("foundMin", prevMinChannelServerVersion),
			zap.String("foundMax", prevMaxChannelServerVersion),
			zap.String("foundChartsAnchor", baseChartsAnchorName))
	}
	logger.Info("Based on previous release",
		zap.String("version", lastReleaseVersionString),
		zap.String("minChannelServerVersion", prevMinChannelServerVersion),
		zap.String("maxChannelServerVersion", prevMaxChannelServerVersion),
		zap.String("chartsAnchorToMergeFrom", baseChartsAnchorName))

	// --- 4. Define and Construct the New Release Node ('v1.21.6+rke2r1') ---
	newReleaseVersion := "v1.21.6+rke2r1"

	// Sanitize the version string for anchor names to be strictly alphanumeric for yaml.v3
	sanitizedVersionForAnchor := strictlyAlphanumeric(newReleaseVersion) // e.g., "v1216rke2r1"

	newServerArgsAnchorName := "serverArgs" + sanitizedVersionForAnchor // e.g., serverArgsv1216rke2r1
	newChartsAnchorName := "charts" + sanitizedVersionForAnchor         // e.g., chartsv1216rke2r1
	logger.Debug("Sanitized anchor names for new release",
		zap.String("newServerArgsAnchor", newServerArgsAnchorName),
		zap.String("newChartsAnchor", newChartsAnchorName))

	chartsToUpdateInNewRelease := map[string]map[string]string{
		"rke2-cilium": {"repo": "rancher-rke2-charts", "version": "1.19.000"},
		"rke2-canal":  {"repo": "rancher-rke2-charts", "version": "v3.31.0"},
	}

	newReleaseNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	var newReleaseContent []*yaml.Node
	newReleaseContent = append(newReleaseContent, createScalarNode("version"), createScalarNode(newReleaseVersion))
	newReleaseContent = append(newReleaseContent, createScalarNode("minChannelServerVersion"), createScalarNode(prevMinChannelServerVersion))
	newReleaseContent = append(newReleaseContent, createScalarNode("maxChannelServerVersion"), createScalarNode(prevMaxChannelServerVersion))
	serverArgsValueNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Anchor: newServerArgsAnchorName} // Empty map with anchor
	newReleaseContent = append(newReleaseContent, createScalarNode("serverArgs"), serverArgsValueNode)

	chartsValueMapNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Anchor: newChartsAnchorName}
	var chartsContent []*yaml.Node
	chartsContent = append(chartsContent,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"},
		&yaml.Node{Kind: yaml.AliasNode, Value: baseChartsAnchorName}, // Alias value is the name of the anchor
	)
	for chartName, details := range chartsToUpdateInNewRelease {
		chartsContent = append(chartsContent,
			createScalarNode(chartName),
			createChartEntryNode(details["repo"], details["version"]),
		)
	}
	chartsValueMapNode.Content = chartsContent
	newReleaseContent = append(newReleaseContent, createScalarNode("charts"), chartsValueMapNode)
	newReleaseNode.Content = newReleaseContent

	// --- 5. Append the New Release Node ---
	releasesSeqNode.Content = append(releasesSeqNode.Content, newReleaseNode)
	logger.Debug("Appended new release to the releases sequence in memory", zap.String("newVersion", newReleaseVersion))

	// --- 6. Marshal and Write ---
	outputBytes, err := yaml.Marshal(&rootNode)
	if err != nil {
		logger.Fatal("Failed to marshal YAML", zap.Error(err))
	}

	err = os.WriteFile(outputFile, outputBytes, 0644)
	if err != nil {
		logger.Fatal("Failed to write updated YAML to file", zap.String("file", outputFile), zap.Error(err))
	}

	logger.Info("Successfully added new release and wrote to file",
		zap.String("newVersion", newReleaseVersion),
		zap.String("outputFile", outputFile))
	fmt.Printf("Review %s to see the changes.\n", outputFile) // Keep a simple fmt.Printf for final user instruction
}
