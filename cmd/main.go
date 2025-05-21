package main

import (
	"fmt"
	"log"
	"os"
	"strings" // Added for strings.Contains

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

// ... (keep your helper functions: createStringNode, createMappingValue, createChartMapNode)
func createStringNode(value string) *ast.StringNode {
	stringToken := token.String(value, token.DoubleQuoteType.String(), nil)
	return ast.String(stringToken)
}

func createMappingValue(keyName string, valueNode ast.Node) *ast.MappingValueNode {
	return &ast.MappingValueNode{
		Key:   createStringNode(keyName),
		Value: valueNode,
	}
}

func createChartMapNode(repo, version string) *ast.MappingNode {
	return &ast.MappingNode{
		Values: []*ast.MappingValueNode{
			createMappingValue("repo", createStringNode(repo)),
			createMappingValue("version", createStringNode(version)),
		},
	}
}

func main() {
	inputFile := "channels-rke2.yaml"
	outputFile := "channels-rke2.output.yaml"

	yamlBytes, err := os.ReadFile(inputFile)
	if err != nil {
		log.Fatalf("Failed to read YAML file %s: %v", inputFile, err)
	}

	file, err := parser.ParseBytes(yamlBytes, parser.ParseComments)
	if err != nil {
		log.Fatalf("Failed to parse YAML: %v", err)
	}
	if len(file.Docs) == 0 {
		log.Fatal("No documents found in YAML file")
	}
	docBody := file.Docs[0].Body

	newReleaseVersion := "v1.21.6+rke2r1"
	newServerArgsAnchorName := "serverArgs-" + newReleaseVersion
	newChartsAnchorName := "charts-" + newReleaseVersion
	chartsToUpdateInNewRelease := map[string]map[string]string{
		"rke2-cilium": {"repo": "rancher-rke2-charts", "version": "1.19.000"},
		"rke2-canal":  {"repo": "rancher-rke2-charts", "version": "v3.31.0"},
	}

	var prevMinChannelServerVersion, prevMaxChannelServerVersion, baseChartsAnchorName string
	var lastReleaseVersionString string

	releasesArrPathString := "$.releases"
	releasesArrPath, err := yaml.PathString(releasesArrPathString)
	if err != nil {
		log.Fatalf("Failed to create path for '%s': %v", releasesArrPathString, err)
	}
	releasesNodeInterface, err := releasesArrPath.ReadNode(docBody)
	if err != nil {
		log.Fatalf("Failed to read node at path '%s': %v", releasesArrPathString, err)
	}
	releasesSeqNode, ok := releasesNodeInterface.(*ast.SequenceNode)
	if !ok {
		log.Fatalf("Node at path '%s' is not a sequence. Got: %T", releasesArrPathString, releasesNodeInterface)
	}
	if len(releasesSeqNode.Values) == 0 {
		log.Fatal("'releases' array is empty.")
	}
	lastReleaseNode := releasesSeqNode.Values[len(releasesSeqNode.Values)-1]
	lastReleaseMap, ok := lastReleaseNode.(*ast.MappingNode)
	if !ok {
		log.Fatalf("Last element in 'releases' is not a map.")
	}

	for _, mvNode := range lastReleaseMap.Values {
		keyNode, _ := mvNode.Key.(*ast.StringNode)
		currentKeyName := keyNode.Value
		switch currentKeyName {
		case "version":
			if v, ok := mvNode.Value.(*ast.StringNode); ok {
				lastReleaseVersionString = v.Value
			}
		case "minChannelServerVersion":
			if v, ok := mvNode.Value.(*ast.StringNode); ok {
				prevMinChannelServerVersion = v.Value
			}
		case "maxChannelServerVersion":
			if v, ok := mvNode.Value.(*ast.StringNode); ok {
				prevMaxChannelServerVersion = v.Value
			}
		case "charts":
			if anchorNode, ok := mvNode.Value.(*ast.AnchorNode); ok {
				if nameNode, ok := anchorNode.Name.(*ast.StringNode); ok && nameNode.Value != "" {
					baseChartsAnchorName = nameNode.Value
					log.Printf("DEBUG: Extracted baseChartsAnchorName: '%s'", baseChartsAnchorName)
				}
			} else {
				log.Printf("WARNING: 'charts' in release '%s' is NOT an ast.AnchorNode. Type: %T.", lastReleaseVersionString, mvNode.Value)
			}
		}
	}

	if prevMinChannelServerVersion == "" || prevMaxChannelServerVersion == "" || baseChartsAnchorName == "" {
		log.Fatalf("Could not extract required fields. min='%s', max='%s', baseChartsAnchor='%s'", prevMinChannelServerVersion, prevMaxChannelServerVersion, baseChartsAnchorName)
	}
	fmt.Printf("Based on previous release ('%s'): minChannel=%s, maxChannel=%s, Charts Anchor to merge=* %s\n", lastReleaseVersionString, prevMinChannelServerVersion, prevMaxChannelServerVersion, baseChartsAnchorName)

	newReleaseValues := []*ast.MappingValueNode{
		createMappingValue("version", createStringNode(newReleaseVersion)),
		createMappingValue("minChannelServerVersion", createStringNode(prevMinChannelServerVersion)),
		createMappingValue("maxChannelServerVersion", createStringNode(prevMaxChannelServerVersion)),
		createMappingValue("serverArgs", &ast.AnchorNode{Name: createStringNode(newServerArgsAnchorName), Value: &ast.MappingNode{}}),
	}
	chartsContentValues := []*ast.MappingValueNode{
		{Key: &ast.MergeKeyNode{}, Value: &ast.AliasNode{Value: createStringNode(baseChartsAnchorName)}},
	}
	for name, details := range chartsToUpdateInNewRelease {
		chartsContentValues = append(chartsContentValues, createMappingValue(name, createChartMapNode(details["repo"], details["version"])))
	}
	newReleaseValues = append(newReleaseValues, createMappingValue("charts", &ast.AnchorNode{Name: createStringNode(newChartsAnchorName), Value: &ast.MappingNode{Values: chartsContentValues}}))
	newReleaseNode := &ast.MappingNode{Values: newReleaseValues}

	// --- 6. Modify the AST ---
	originalCount := len(releasesSeqNode.Values)
	releasesSeqNode.Values = append(releasesSeqNode.Values, newReleaseNode)
	log.Printf("DEBUG: Appended new release. Original count: %d, New count: %d", originalCount, len(releasesSeqNode.Values))
	// (In-memory check logs - keep them if you want, they should still pass)
	if len(releasesSeqNode.Values) > originalCount {
		lastAdded := releasesSeqNode.Values[len(releasesSeqNode.Values)-1].(*ast.MappingNode).Values[0].Value.(*ast.StringNode) // Quick check for version of last added
		log.Printf("DEBUG: Version of the programmatically added node (read back from AST): '%s'", lastAdded.Value)
		if lastAdded.Value == newReleaseVersion {
			log.Printf("DEBUG: AST modification appears successful in memory.")
		} else {
			log.Printf("ERROR: Added node version mismatch!")
		}
	}

	// --- Attempt to use ReplaceWithNode to "refresh" the releases sequence in the AST ---
	log.Println("DEBUG: Attempting ReplaceWithNode on the 'releases' sequence with its modified self.")
	err = releasesArrPath.ReplaceWithNode(file, releasesSeqNode) // releasesArrPath was "$.releases"
	if err != nil {
		// If ReplaceWithNode fails, log it but proceed to see if serialization works anyway
		log.Printf("ERROR: ReplaceWithNode for 'releases' array failed: %v. Proceeding with serialization...", err)
	} else {
		log.Println("DEBUG: ReplaceWithNode completed.")
	}
	// --- End of ReplaceWithNode attempt ---

	fmt.Printf("Prepared new release '%s' for inclusion in AST.\n", newReleaseVersion)

	// --- 7. Serialize the Modified AST back to YAML ---
	log.Println("DEBUG: About to call file.String() to serialize AST.")
	docBodyYAML := docBody.String()
	if strings.Contains(docBodyYAML, newReleaseVersion) {
		log.Printf("IMPORTANT DEBUG: newReleaseVersion ('%s') IS PRESENT in direct docBody.String() output!", newReleaseVersion)
	} else {
		log.Printf("IMPORTANT DEBUG: newReleaseVersion ('%s') IS MISSING from direct docBody.String() output!", newReleaseVersion)
	}
	outputYAML := file.String()
	if strings.Contains(outputYAML, newReleaseVersion) {
		log.Printf("DEBUG: newReleaseVersion ('%s') IS present in file.String() output.", newReleaseVersion)
	} else {
		log.Printf("DEBUG: newReleaseVersion ('%s') IS MISSING from file.String() output.", newReleaseVersion)
	}

	err = os.WriteFile(outputFile, []byte(outputYAML), 0644)
	if err != nil {
		log.Fatalf("Failed to write updated YAML to %s: %v", outputFile, err)
	}
	fmt.Printf("Successfully added new release '%s' and wrote to %s\n", newReleaseVersion, outputFile)
	fmt.Printf("Review %s to see the changes.\n", outputFile)
}
