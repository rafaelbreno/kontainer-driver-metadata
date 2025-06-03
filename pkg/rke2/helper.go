package rke2

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
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
		return 0, Release{}, fmt.Errorf("not a mapping node in '%s' release", prevVersion)
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
					zap.L().Debug("Found agentArgs anchor from last release", zap.String("anchorName", release.agentArgsAnchor))
				} else if valueNode.Kind == yaml.AliasNode {
					release.agentArgsAnchor = valueNode.Value
					zap.L().Debug("Found agentArgs alias from last release",
						zap.String("releaseVersion", prevVersion),
						zap.String("aliasName", valueNode.Value))
				} else {
					zap.L().Warn("'agentArgs' in last release does not have an anchor or is not a map.",
						zap.String("releaseVersion", prevVersion),
						zap.Any("nodeKind", valueNode.Kind),
						zap.String("anchorValue", valueNode.Anchor))
				}
			case "serverArgs":
				fmt.Println("DocumentNode", yaml.DocumentNode)
				fmt.Println("SequenceNode", yaml.SequenceNode)
				fmt.Println("MappingNode", yaml.MappingNode)
				fmt.Println("ScalarNode", yaml.ScalarNode)
				fmt.Println("AliasNode", yaml.AliasNode)
				fmt.Println("valueNode.Kind", valueNode.Kind)
				//dd(valueNode.Value)
				if valueNode.Kind == yaml.MappingNode && valueNode.Anchor != "" {
					release.serverArgsAnchor = valueNode.Anchor // This anchor name is from the file, assume it's valid
					zap.L().Debug("Found serverArgs anchor from last release", zap.String("anchorName", release.serverArgsAnchor))
				} else if valueNode.Kind == yaml.AliasNode {
					release.serverArgsAnchor = valueNode.Value
					zap.L().Debug("Found serverArgs alias from last release",
						zap.String("releaseVersion", prevVersion),
						zap.String("aliasName", valueNode.Value))
				} else {
					zap.L().Warn("'serverArgs' in last release does not have an anchor or is not a map.",
						zap.String("releaseVersion", prevVersion),
						zap.Any("nodeKind", valueNode.Kind),
						zap.String("anchorValue", valueNode.Anchor))
				}
			case "charts":
				if valueNode.Kind == yaml.MappingNode && valueNode.Anchor != "" {
					release.chartsAnchor = valueNode.Anchor // This anchor name is from the file, assume it's valid
					zap.L().Debug("Found charts anchor from last release", zap.String("anchorName", release.chartsAnchor))
				} else {
					zap.L().Warn("'charts' in last release does not have an anchor or is not a map.",
						zap.String("releaseVersion", prevVersion),
						zap.Any("nodeKind", valueNode.Kind),
						zap.String("anchorValue", valueNode.Anchor))
				}
			}
		}
	}
	return prevReleasePos, release, nil
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
						if valueNode.Value == version {
							return i, nil
						}
					}
				}
			}
		}
	}
	return -1, fmt.Errorf("unable to find release '%s'", version)
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

func dd(v any) {
	pp(v)
	os.Exit(1)
}

func pp(v any) {
	if k, ok := v.(yaml.Node); ok {
		b, err := yaml.Marshal(k)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(b))
		return
	}

	if k, ok := v.(*yaml.Node); ok {
		b, err := yaml.Marshal(k)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(b))
		return
	}

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}
