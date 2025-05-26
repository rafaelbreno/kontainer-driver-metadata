package rke2

import (
	"bytes"
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

const (
	inputFile  = "channels-rke2.yaml"
	outputFile = "channels-rke2.output.yaml"
)

func UpdateRKE2(versions ...string) error {
	s := &S{}
	if err := s.parseYaml(inputFile); err != nil {
		return err
	}

	if err := s.setReleasesNode(); err != nil {
		return err
	}

	releases, err := s.getReleases(versions)
	if err != nil {
		return err
	}

	for _, release := range releases {
		if err := s.addRelease(release); err != nil {
			return err
		}
	}

	b, err := s.Bytes()

	err = os.WriteFile(outputFile, b, 0644)
	if err != nil {
		return err
	}

	return nil
}

type (
	S struct {
		channels        RKE2Channels
		rootNode        yaml.Node
		rootDoc         *yaml.Node
		releasesSeqNode *yaml.Node
	}
)

func (s *S) Bytes() ([]byte, error) {
	outputBytes, err := yaml.Marshal(s.rootNode)
	if err != nil {
		return nil, err
	}

	outputBytes = bytes.ReplaceAll(outputBytes, []byte("!!merge "), nil)
	outputBytes = bytes.ReplaceAll(outputBytes, []byte(" {}"), nil)
}

func (s *S) addRelease(release Release) error {
	var newReleaseContent []*yaml.Node
	newReleaseContent = append(newReleaseContent, createScalarNode("version"), createScalarNode(release.Version))
	prevReleasePos, prevRelease, err := getPreviousRelease(s.releasesSeqNode, release.Version)

	if err != nil {
		return err
	}

	newReleaseContent = append(newReleaseContent, createScalarNode("minChannelServerVersion"), createScalarNode(prevRelease.MinChannelServerVersion))
	newReleaseContent = append(newReleaseContent, createScalarNode("maxChannelServerVersion"), createScalarNode(prevRelease.MaxChannelServerVersion))

	sanitizedVersionForAnchor := strictlyAlphanumeric(release.Version) // e.g., "v1216rke2r1"

	// defining charts
	{
		newChartsAnchorName := "charts" + sanitizedVersionForAnchor
		chartsContent := []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"},
			{Kind: yaml.AliasNode, Value: prevRelease.chartsAnchor}, // Alias value is the name of the anchor
		}
		for chartName, chart := range release.Charts {
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
			{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"},
			{Kind: yaml.AliasNode, Value: prevRelease.serverArgsAnchor}, // Alias value is the name of the anchor
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
			{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"},
			{Kind: yaml.AliasNode, Value: prevRelease.agentArgsAnchor}, // Alias value is the name of the anchor
		}
		agentArgsValueMapNode := &yaml.Node{
			Kind:    yaml.MappingNode,
			Tag:     "!!map",
			Anchor:  newAgentArgsAnchorName,
			Content: agentArgsContent,
		}
		newReleaseContent = append(newReleaseContent, createScalarNode("agentArgs"), agentArgsValueMapNode)
	}

	newReleaseNode := &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     "!!map",
		Content: newReleaseContent,
	}

	s.releasesSeqNode.Content = slices.Insert(s.releasesSeqNode.Content, prevReleasePos+1, newReleaseNode)

	return nil
}

func (s *S) getReleases(versions []string) ([]Release, error) {
	releases := []Release{}
	for _, version := range versions {
		prevVersion := getPreviousVersion(version)

		chart, err := GetUpdatedCharts(version, prevVersion)
		if err != nil {
			return nil, err
		}
		releases = append(releases, Release{
			Version:     version,
			Charts:      chart,
			prevVersion: prevVersion,
		})

	}
	return releases, nil
}

func (s *S) parseYaml(filename string) error {
	yamlBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil
	}

	var rke2channels RKE2Channels
	if err = yaml.Unmarshal(yamlBytes, &rke2channels); err != nil {
		return nil
	}

	s.channels = rke2channels

	var rootNode yaml.Node
	err = yaml.Unmarshal(yamlBytes, &rootNode)
	if err != nil {
		return nil
	}

	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return fmt.Errorf("expected a YAML document node at the root")
	}

	s.rootNode = rootNode
	s.rootDoc = rootNode.Content[0]

	return nil
}

func (s *S) setReleasesNode() error {
	var releasesSeqNode *yaml.Node

	if s.rootDoc.Kind == yaml.MappingNode {
		// docContent.Content for a MappingNode is a flat list: [key1, value1, key2, value2, ...]
		// so here we need to iterate like i+=2
		for i := 0; i < len(s.rootDoc.Content); i += 2 {
			keyNode := s.rootDoc.Content[i]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "releases" {
				releasesSeqNode = s.rootDoc.Content[i+1]
				break
			}
		}
	}

	if releasesSeqNode == nil || releasesSeqNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("could not find 'releases' sequence in YAML or it's not a sequence")
	}

	if len(releasesSeqNode.Content) == 0 {
		return fmt.Errorf("'releases' sequence is empty, cannot determine the last release")
	}

	s.releasesSeqNode = releasesSeqNode

	return nil

}
