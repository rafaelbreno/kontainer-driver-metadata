package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"

	"go.uber.org/zap"
	yaml "gopkg.in/yaml.v3"
)

var logger *zap.Logger

func readAndParseYaml(filename string) (yaml.Node, *yaml.Node, error) {
	yamlBytes, err := os.ReadFile(filename)
	if err != nil {
		logger.Error("Failed to read YAML file", zap.String("file", filename), zap.Error(err))
		return yaml.Node{}, nil, err
	}

	var rootNode yaml.Node
	err = yaml.Unmarshal(yamlBytes, &rootNode)
	if err != nil {
		logger.Error("Failed to unmarshal YAML", zap.Error(err))
		return yaml.Node{}, nil, err
	}

	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		logger.Error("Expected a YAML document node at the root.")
		return yaml.Node{}, nil, err
	}

	return rootNode, rootNode.Content[0], nil
}

func getReleaseSeqNode(doc *yaml.Node) (*yaml.Node, error) {
	if doc == nil {
		return nil, fmt.Errorf("Provided doc content is nil")
	}
	var releasesSeqNode *yaml.Node
	if doc.Kind == yaml.MappingNode {
		// docContent.Content for a MappingNode is a flat list: [key1, value1, key2, value2, ...]
		// so here we need to iterate like i+=2
		for i := 0; i < len(doc.Content); i += 2 {
			keyNode := doc.Content[i]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "releases" {
				releasesSeqNode = doc.Content[i+1]
				break
			}
		}
	}

	if releasesSeqNode == nil || releasesSeqNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("Could not find 'releases' sequence in YAML or it's not a sequence.")
	}
	if len(releasesSeqNode.Content) == 0 {
		return nil, fmt.Errorf("'releases' sequence is empty. Cannot determine the last release.")
	}
	return releasesSeqNode, nil
}

func main() {
	// Initialize Zap logger
	var initErr error
	logger, initErr = zap.NewDevelopment()
	if initErr != nil {
		fmt.Printf("Failed to initialize zap logger: %v\n", initErr)
		os.Exit(1)
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	inputFile := "channels-rke2.yaml"
	outputFile := "channels-rke2.output.yaml"

	// --- 1. Read and Parse the YAML file ---
	rootNode, docContent, err := readAndParseYaml(inputFile)
	if err != nil {
		log.Fatal(
			"Failed to read and parse file",
			zap.String("filename", inputFile),
			zap.Error(err),
		)
	}

	// --- 2. Navigate to the 'releases' sequence ---
	releasesSeqNode, err := getReleaseSeqNode(docContent)

	newReleases := []Release{
		{
			Version: "v1.21.5+rke2r1",
			Charts: map[string]Chart{
				"rke2-cilium": Chart{
					Repo:    "rancher-rke2-charts",
					Version: "1.11.000",
				},
			},
		},
		{
			Version: "v1.22.5+rke2r1",
			Charts: map[string]Chart{
				"rke2-cilium": Chart{
					Repo:    "rancher-rke2-charts",
					Version: "1.11.000",
				},
			},
		},
		{
			Version: "v1.23.5+rke2r1",
			Charts: map[string]Chart{
				"rke2-cilium": Chart{
					Repo:    "rancher-rke2-charts",
					Version: "1.11.000",
				},
			},
		},
	}

	if err := update(releasesSeqNode, newReleases...); err != nil {
		logger.Fatal("Error updating releases sequence node", zap.Error(err))
	}

	outputBytes, err := yaml.Marshal(&rootNode)
	if err != nil {
		logger.Fatal("Failed to marshal YAML", zap.Error(err))
	}

	outputBytes = bytes.ReplaceAll(outputBytes, []byte("!!merge "), nil)
	outputBytes = bytes.ReplaceAll(outputBytes, []byte(" {}"), nil)

	err = os.WriteFile(outputFile, outputBytes, 0644)
	if err != nil {
		logger.Fatal("Failed to write updated YAML to file", zap.String("file", outputFile), zap.Error(err))
	}

	os.Exit(1)

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
	outputBytes, err = yaml.Marshal(&rootNode)
	if err != nil {
		logger.Fatal("Failed to marshal YAML", zap.Error(err))
	}

	outputBytes = bytes.ReplaceAll(outputBytes, []byte("!!merge "), nil)
	outputBytes = bytes.ReplaceAll(outputBytes, []byte(" {}"), nil)

	err = os.WriteFile(outputFile, outputBytes, 0644)
	if err != nil {
		logger.Fatal("Failed to write updated YAML to file", zap.String("file", outputFile), zap.Error(err))
	}

	logger.Info("Successfully added new release and wrote to file",
		zap.String("newVersion", newReleaseVersion),
		zap.String("outputFile", outputFile))
	fmt.Printf("Review %s to see the changes.\n", outputFile) // Keep a simple fmt.Printf for final user instruction
}

type (
	Release struct {
		Version                 string           `yaml:"version"`
		MinChannelServerVersion string           `yaml:"minChannelServerVersion"`
		MaxChannelServerVersion string           `yaml:"maxChannelServerVersion"`
		ServerArgs              map[string]Arg   `yaml:"serverArgs"`
		serverArgsAnchor        string           `yaml:"-"`
		AgentArgs               map[string]Arg   `yaml:"agentArgs"`
		agentArgsAnchor         string           `yaml:"-"`
		Charts                  map[string]Chart `yaml:"charts"`
		chartsAnchor            string           `yaml:"-"`
	}

	Arg struct {
		Default  string   `yaml:"default"`
		Type     string   `yaml:"type"`
		Options  []string `yaml:"options"`
		Nullable bool     `yaml:"nullable"`
	}

	Chart struct {
		Repo    string `yaml:"repo"`
		Version string `yaml:"version"`
	}
)

func getPreviousVersion(version string) string {
	// TODO:
	// 1. Support (v1.33.0+rke2r1) -> (v1.32.9+rke2r1)
	// 2. Support (v1.33.0+rke2r2) -> (v1.33.0+rke2r1)

	vs := strings.Split(version, ".")
	patchStr := vs[len(vs)-1]
	patchStr = strings.TrimSuffix(patchStr, "+rke2r1")
	patch, err := strconv.Atoi(patchStr)
	if err != nil {
		zap.L().Fatal(
			"Error converting the patch to number",
			zap.String("version", version),
			zap.Error(err),
		)
	}

	return fmt.Sprintf("%s.%s.%d+rke2r1", vs[0], vs[1], patch-1)
}

func pp(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		zap.L().Fatal("Error mashal", zap.Error(err))
	}
	fmt.Println(string(b))
}
func dd(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		zap.L().Fatal("Error mashal", zap.Error(err))
	}
	fmt.Println(string(b))
	os.Exit(1)
}

func getPreviousReleasePos(releaseNode *yaml.Node, version string) (int, error) {
	//pp(*releaseNode)
	//os.Exit(1)
	for i := 0; i < len(releaseNode.Content); i++ {
		node := releaseNode.Content[i]
		if node.Kind == yaml.MappingNode {
			for j := 0; j < len(node.Content); j += 2 {
				keyNode := node.Content[j]
				valueNode := node.Content[j+1]
				if keyNode.Kind == yaml.ScalarNode {
					switch keyNode.Value {
					case "version":
						fmt.Printf("%s == %s\n", valueNode.Value, version)
						if valueNode.Value == version {
							return i, nil
						}
					}
				}
			}
		}
	}
	return -1, fmt.Errorf("Unable to find release '%s'", version)
}

func getPreviousRelease(releaseNode *yaml.Node, version string) (int, Release, error) {
	prevVersion := getPreviousVersion(version)
	prevReleasePos, err := getPreviousReleasePos(releaseNode, prevVersion)
	if err != nil {
		return 0, Release{}, err
	}
	release := Release{}
	node := releaseNode.Content[prevReleasePos]
	//dd(node)

	if node.Kind != yaml.MappingNode {
		return 0, Release{}, fmt.Errorf("Not a mapping node in '%s' release", prevVersion)
	}
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode {
			switch keyNode.Value {
			case "version":
				release.Version = valueNode.Value
			case "minChannelServerVersion":
				release.MinChannelServerVersion = valueNode.Value
			case "maxChannelServerVersion":
				release.MaxChannelServerVersion = valueNode.Value
			case "agentArgs":
				if valueNode.Kind == yaml.MappingNode && valueNode.Anchor != "" {
					release.agentArgsAnchor = valueNode.Anchor // This anchor name is from the file, assume it's valid
					logger.Debug("Found agentArgs anchor from last release", zap.String("anchorName", release.agentArgsAnchor))
				} else {
					logger.Warn("'agentArgs' in last release does not have an anchor or is not a map.",
						zap.String("releaseVersion", prevVersion),
						zap.Any("nodeKind", valueNode.Kind),
						zap.String("anchorValue", valueNode.Anchor))
				}
			case "serverArgs":
				if valueNode.Kind == yaml.MappingNode && valueNode.Anchor != "" {
					release.serverArgsAnchor = valueNode.Anchor // This anchor name is from the file, assume it's valid
					logger.Debug("Found serverArgs anchor from last release", zap.String("anchorName", release.serverArgsAnchor))
				} else {
					logger.Warn("'serverArgs' in last release does not have an anchor or is not a map.",
						zap.String("releaseVersion", prevVersion),
						zap.Any("nodeKind", valueNode.Kind),
						zap.String("anchorValue", valueNode.Anchor))
				}
			case "charts":
				if valueNode.Kind == yaml.MappingNode && valueNode.Anchor != "" {
					release.chartsAnchor = valueNode.Anchor // This anchor name is from the file, assume it's valid
					logger.Debug("Found charts anchor from last release", zap.String("anchorName", release.chartsAnchor))
				} else {
					logger.Warn("'charts' in last release does not have an anchor or is not a map.",
						zap.String("releaseVersion", prevVersion),
						zap.Any("nodeKind", valueNode.Kind),
						zap.String("anchorValue", valueNode.Anchor))
				}
			}
		}
	}
	return prevReleasePos, release, nil
}

func update(releaseNode *yaml.Node, newReleases ...Release) error {
	releaseNodes := make(map[string][]*yaml.Node, len(newReleases))
	if len(newReleases) == 0 {
		return fmt.Errorf("releases list not provided")
	}

	for _, newRelease := range newReleases {
		var newReleaseContent []*yaml.Node

		newReleaseContent = append(newReleaseContent, createScalarNode("version"), createScalarNode(newRelease.Version))

		prevReleasePos, prevRelease, err := getPreviousRelease(releaseNode, newRelease.Version)
		if err != nil {
			zap.L().Fatal(
				"Unable to retrieve previous version.",
				zap.String("version", newRelease.Version),
				zap.Error(err),
			)
			return err
		}

		newReleaseContent = append(newReleaseContent, createScalarNode("minChannelServerVersion"), createScalarNode(prevRelease.MinChannelServerVersion))
		newReleaseContent = append(newReleaseContent, createScalarNode("maxChannelServerVersion"), createScalarNode(prevRelease.MaxChannelServerVersion))

		sanitizedVersionForAnchor := strictlyAlphanumeric(newRelease.Version) // e.g., "v1216rke2r1"

		// defining charts
		{
			newChartsAnchorName := "charts" + sanitizedVersionForAnchor
			chartsContent := []*yaml.Node{
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"},
				&yaml.Node{Kind: yaml.AliasNode, Value: prevRelease.chartsAnchor}, // Alias value is the name of the anchor
			}
			for chartName, chart := range newRelease.Charts {
				chartsContent = append(chartsContent,
					createScalarNode(chartName),
					createChartEntryNode(chart.Repo, chart.Version),
				)
			}
			chartsValueMapNode := &yaml.Node{
				Kind:    yaml.MappingNode,
				Tag:     "!!map",
				Anchor:  newChartsAnchorName,
				Content: chartsContent,
			}
			newReleaseContent = append(newReleaseContent, createScalarNode("charts"), chartsValueMapNode)
		}

		// defining serverArgs
		{
			newServerArgsAnchorName := "serverArgs" + sanitizedVersionForAnchor // e.g., serverArgsv1216rke2r1
			serverArgsContent := []*yaml.Node{
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"},
				&yaml.Node{Kind: yaml.AliasNode, Value: prevRelease.serverArgsAnchor}, // Alias value is the name of the anchor
			}
			serverArgsValueMapNode := &yaml.Node{
				Kind:    yaml.MappingNode,
				Tag:     "!!map",
				Anchor:  newServerArgsAnchorName,
				Content: serverArgsContent,
			}
			newReleaseContent = append(newReleaseContent, createScalarNode("serverArgs"), serverArgsValueMapNode)
		}

		// defining agentArgs
		{
			newAgentArgsAnchorName := "agentArgs" + sanitizedVersionForAnchor // e.g., agentArgsv1216rke2r1
			agentArgsContent := []*yaml.Node{
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"},
				&yaml.Node{Kind: yaml.AliasNode, Value: prevRelease.agentArgsAnchor}, // Alias value is the name of the anchor
			}
			agentArgsValueMapNode := &yaml.Node{
				Kind:    yaml.MappingNode,
				Tag:     "!!map",
				Anchor:  newAgentArgsAnchorName,
				Content: agentArgsContent,
			}
			newReleaseContent = append(newReleaseContent, createScalarNode("agentArgs"), agentArgsValueMapNode)
		}

		releaseNodes[newRelease.Version] = newReleaseContent

		newReleaseNode := &yaml.Node{
			Kind:    yaml.MappingNode,
			Tag:     "!!map",
			Content: newReleaseContent,
		}

		// appending the content

		releaseNode.Content = slices.Insert(releaseNode.Content, prevReleasePos+1, newReleaseNode)
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

// Helper to create a sequence node (array) from a slice of string values
func createSequenceNode(values []string) *yaml.Node {
	sequenceNode := &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq", // Explicitly a sequence
		// Style: 0, // Default block style. Use yaml.FlowStyle for [item1, item2]
	}

	// Populate the Content of the sequence node
	for _, valStr := range values {
		itemNode := createScalarNode(valStr) // Each item in the array is a scalar node
		sequenceNode.Content = append(sequenceNode.Content, itemNode)
	}
	return sequenceNode
}

func createArgsEntryNode(arg Arg) *yaml.Node {
	content := []*yaml.Node{
		createScalarNode("default"), createScalarNode(arg.Default),
		createScalarNode("type"), createScalarNode(arg.Type),
	}

	if arg.Nullable {
		content = append(content,
			createScalarNode("nullable"), createScalarNode(strconv.FormatBool(arg.Nullable)),
		)
	}

	if len(arg.Options) > 0 {
		content = append(content,
			createScalarNode("options"), createSequenceNode(arg.Options),
		)
	}

	return &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     "!!map",
		Content: content,
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
