package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

type (
	ChannelsRKE2 struct {
		Channels    []Channel `yaml:"channels"`
		AppDefaults []App     `yaml:"appDefaults"`
		Releases    []Release `yaml:"releases"`
	}

	Channel struct {
		Name   string `yaml:"name"`
		Latest string `yaml:"latest"`
	}

	App struct {
		AppName  string    `yaml:"appName"`
		Defaults []Default `yaml:"defaults"`
	}
	Default struct {
		AppVersion     string `yaml:"appVersion"`
		DefaultVersion string `yaml:"defaultVersion"`
	}

	Release struct {
		Version                 string `yaml:"version"`
		minChannelServerVersion string `yaml:"minChannelServerVersion"`
		maxChannelServerVersion string `yaml:"maxChannelServerVersion"`
		//ServerArgs              any    `yaml:"serverArgs"`
		Charts map[string]Chart `yaml:"charts"`
	}

	Chart struct {
		Repo    string `yaml:"repo"`
		Version string `yaml:"version"`
	}
)

func (r Release) SanitizeVersion() string {
	regexStr := `[^a-zA-Z0-9_-]`
	re, err := regexp.Compile(regexStr)
	if err != nil {
		log.Fatalf("Failed to parse regex string '%s': %v", regexStr, err)
	}
	return re.ReplaceAllString(r.Version, "-")

}

func (r Release) ServerArgsAnchorName() string {
	return "serverArgs-" + r.SanitizeVersion()
}

func (r Release) AgentArgsAnchorName() string {
	return "agentArgs-" + r.SanitizeVersion()
}

func (r Release) ChartsAnchorName() string {
	return "charts-" + r.SanitizeVersion()
}

const (
	channelsRKE2Filename = "channels-rke2.yaml"
	channelsK3sFilename  = "channels.yaml"
)

var (
	releasesMap             map[string]map[string]Release = make(map[string]map[string]Release)
	majorMinorCurrentCharts map[string]map[string]string  = make(map[string]map[string]string)
)

func main() {
	b, err := os.ReadFile(channelsRKE2Filename)
	if err != nil {
		log.Fatalf("Failed to read '%s' file: %v", channelsRKE2Filename, err)
	}

	file, err := parser.ParseBytes(b, parser.ParseComments)
	if err != nil {
		log.Fatalf("Failed to parse '%s' YAML file: %v", channelsRKE2Filename, err)
	}

	if len(file.Docs) == 0 {
		log.Fatalf("No Document Node found in '%s' YAML file: %v", channelsRKE2Filename, err)
	}

	// If what I found is correct, YAML can be divided in docs, by using:
	// "----"
	// Something that we don't have in `channels-rke2.yaml`,
	// so we can just assume:
	docBody := file.Docs[0].Body
	//releaseToInsert := Release{
	//minChannelServerVersion: "v2.11.0-alpha1",
	//maxChannelServerVersion: "v2.11.99",
	//Version:                 "v1.32.6+rke2r1-GENERATED",
	//Charts: map[string]Chart{
	//"rke2-cilium": {
	//Repo:    "rancher-rke2-charts",
	//Version: "1.18.000-GENERATED",
	//},
	//"rke2-canal": {
	//Repo:    "rancher-rke2-charts",
	//Version: "v3.30.0-GENERATED",
	//},
	//},
	//}

	var prevMinChannelServerVersion, prevMaxChannelServerVersion, baseServerArgsAnchorName string

	// Path to the 'releases' array itself
	releasesPathString := "$.releases"
	releasesArrPath, err := yaml.PathString(releasesPathString)
	if err != nil {
		log.Fatalf("Failed to create path for '%s': %v", releasesPathString, err)
	}

	releasesNodeInterface, err := releasesArrPath.ReadNode(docBody)
	if err != nil {
		log.Fatalf("Failed to read '%s' node: %v", releasesPathString, err)
	}

	releasesSeqNode, ok := releasesNodeInterface.(*ast.SequenceNode)
	if !ok {
		log.Fatalf("Node at '%s' is not a sequence (array) as expected.", releasesPathString)
	}

	if len(releasesSeqNode.Values) == 0 {
		// Handle the case where the releases array might be empty,
		// though your YAML structure implies it won't be.
		log.Fatal("'releases' array is empty. Cannot determine the last release to base the new one on.")
	}

	// Get the last element from the sequence
	lastReleaseNode := releasesSeqNode.Values[len(releasesSeqNode.Values)-1]

	// Ensure the last element is a MappingNode (a map)
	lastReleaseMap, ok := lastReleaseNode.(*ast.MappingNode)
	if !ok {
		log.Fatalf("The last element in the 'releases' array is not a YAML map as expected.")
	}

	// Now, iterate through the key-value pairs of the lastReleaseMap
	for _, valNode := range lastReleaseMap.Values {
		keyNode, ok := valNode.Key.(*ast.StringNode)
		if !ok {
			continue
		}
		switch keyNode.Value {
		case "minChannelServerVersion":
			if v, ok := valNode.Value.(*ast.StringNode); ok {
				prevMinChannelServerVersion = v.Value
			}
		case "maxChannelServerVersion":
			if v, ok := valNode.Value.(*ast.StringNode); ok {
				prevMaxChannelServerVersion = v.Value
			}
		case "serverArgs":
			if anchorNode, ok := valNode.Value.(*ast.AnchorNode); ok {
				if nameNode, ok := anchorNode.Name.(*ast.StringNode); ok {
					baseServerArgsAnchorName = nameNode.Value
				} else {
					log.Printf("Warning: 'serverArgs' in the last release was not directly an ast.AnchorNode. Its type is %T. No base anchor name retrieved for merging.", valNode.Value)
				}
			}
		}
	}

	if prevMinChannelServerVersion == "" || prevMaxChannelServerVersion == "" || baseServerArgsAnchorName == "" {
		log.Fatalf("Could not extract all required fields (minChannelServerVersion, maxChannelServerVersion, serverArgs anchor) from the last release. Last release values found: min='%s', max='%s', anchor='%s'", prevMinChannelServerVersion, prevMaxChannelServerVersion, baseServerArgsAnchorName)
	}

	fmt.Printf("Based on previous release: minChannelServerVersion=%s, maxChannelServerVersion=%s, serverArgs Anchor to merge=* %s\n",
		prevMinChannelServerVersion, prevMaxChannelServerVersion, baseServerArgsAnchorName)

	fmt.Println("Aloo", prevReleasePath.String())

	//_ = github.NewClient(nil)
	//b, err := os.ReadFile(channelsRKE2Filename)
	//if err != nil {
	//panic(err)
	//}

	//var channels ChannelsRKE2

	//if err := yaml.Unmarshal(b, &channels); err != nil {
	//panic(err)
	//}

	//for _, release := range channels.Releases {
	//majorMinor := getMajorMinor(release.Version)
	//if _, ok := releasesMap[majorMinor]; !ok {
	//releasesMap[majorMinor] = map[string]Release{}
	//}

	//releasesMap[majorMinor][release.Version] = release
	//b, err := json.MarshalIndent(release.Charts, "", "	")
	//if err != nil {
	//panic(err)
	//}
	//fmt.Println(string(b))
	//}
}

func getMajorMinor(v string) string {
	strs := strings.Split(v, ".")
	if len(strs) > 2 {
		return fmt.Sprintf("%s.%s", strs[0], strs[1])
	}
	panic("version invalid: " + v)
}
