package rke2

type (
	RKE2Channels struct {
		Releases []Release `yaml:"releases"`
	}

	Release struct {
		Version                 string           `yaml:"version"`
		prevVersion             string           `yaml:"-"`
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
		Repo     string `yaml:"repo,omitempty"`
		Version  string `yaml:"version,omitempty"`
		Filename string `yaml:"filename,omitempty"`
	}
)
