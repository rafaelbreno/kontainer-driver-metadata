package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/google/go-github/v72/github"
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
		Charts any `yaml:"charts"`
	}
)

const (
	channelsRKE2Filename = "channels-rke2.yaml"
	channelsK3sFilename  = "channels.yaml"
)

var (
	releasesMap             map[string]map[string]Release = make(map[string]map[string]Release)
	majorMinorCurrentCharts map[string]map[string]string  = make(map[string]map[string]string)
)

func main() {
	_ = github.NewClient(nil)
	b, err := os.ReadFile(channelsRKE2Filename)
	if err != nil {
		panic(err)
	}

	var channels ChannelsRKE2

	if err := yaml.Unmarshal(b, &channels); err != nil {
		panic(err)
	}

	for _, release := range channels.Releases {
		majorMinor := getMajorMinor(release.Version)
		if _, ok := releasesMap[majorMinor]; !ok {
			releasesMap[majorMinor] = map[string]Release{}
		}

		releasesMap[majorMinor][release.Version] = release
		b, err := json.MarshalIndent(release.Charts, "", "	")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(b))
	}
}

func getMajorMinor(v string) string {
	strs := strings.Split(v, ".")
	if len(strs) > 2 {
		return fmt.Sprintf("%s.%s", strs[0], strs[1])
	}
	panic("version invalid: " + v)
}
