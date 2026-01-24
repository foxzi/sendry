package headers

// Action defines the type of header manipulation
type Action string

const (
	ActionRemove  Action = "remove"
	ActionReplace Action = "replace"
	ActionAdd     Action = "add"
)

// Rule defines a header manipulation rule
type Rule struct {
	Action  Action   `yaml:"action" json:"action"`
	Headers []string `yaml:"headers,omitempty" json:"headers,omitempty"` // For remove action
	Header  string   `yaml:"header,omitempty" json:"header,omitempty"`   // For replace/add
	Value   string   `yaml:"value,omitempty" json:"value,omitempty"`     // For replace/add
}

// Config contains header rules configuration
type Config struct {
	// Global rules applied to all messages
	Global []Rule `yaml:"global,omitempty" json:"global,omitempty"`

	// Per-domain rules
	Domains map[string][]Rule `yaml:"domains,omitempty" json:"domains,omitempty"`
}

// GetRulesForDomain returns rules for a specific domain (global + domain-specific)
func (c *Config) GetRulesForDomain(domain string) []Rule {
	if c == nil {
		return nil
	}

	var rules []Rule

	// Add global rules first
	rules = append(rules, c.Global...)

	// Add domain-specific rules
	if c.Domains != nil {
		if domainRules, ok := c.Domains[domain]; ok {
			rules = append(rules, domainRules...)
		}
	}

	return rules
}

// HasRules returns true if any rules are configured
func (c *Config) HasRules() bool {
	if c == nil {
		return false
	}
	if len(c.Global) > 0 {
		return true
	}
	for _, rules := range c.Domains {
		if len(rules) > 0 {
			return true
		}
	}
	return false
}
