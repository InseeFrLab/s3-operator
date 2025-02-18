package model

type IAM struct {
	Version   string         `json:"Version"`
	Statement []IAMStatement `json:"Statement"`
}

type IAMStatement struct {
	Effect    string              `json:"Effect,omitempty"`
	Principal map[string][]string `json:"Principal,omitempty"`
	Action    []string            `json:"Action,omitempty"`
	Resource  []string            `json:"Resource,omitempty"`
}
