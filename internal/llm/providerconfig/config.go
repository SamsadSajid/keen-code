package providerconfig

type Provider string

type ClientConfig struct {
	Provider            Provider
	APIKey              string
	APIKeyHelper        string
	Model               string
	ThinkingEffort      string
	BaseURL             string
	MaxRetries          int
	ContextWindowTokens int
	Headers             map[string]string
}
