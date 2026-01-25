package main

import (
	"os"
)

type OGProvider struct {
	Name    string
	URL     string
	APIKey  string
	Models  []string // The first is the default.
	APIBase string   // e.g., /v1/openai
}

var ogProviders = []OGProvider{
	{
		Name:    "Gemini (via OpenAI)",
		URL:     "https://generativelanguage.googleapis.com/v1beta",
		APIKey:  os.Getenv("GEMINI_API_KEY"),
		Models:  []string{"gemini-2.5-flash"},
		APIBase: "/v1/gemini",
	},
	{
		Name:    "OpenAI",
		URL:     "https://api.openai.com/v1",
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Models:  []string{"gpt-4o"},
		APIBase: "/v1/openai",
	},
	{
		Name:    "Groq",
		URL:     "https://api.groq.com/openai/v1",
		APIKey:  os.Getenv("GROQ_API_KEY"),
		Models:  []string{"llama-3.1-8b-instant"},
		APIBase: "/v1/groq",
	},
	{
		Name:    "OG-Test",
		URL:     "http://og:11434", // URL for the gateway itself for testing
		APIKey:  "OG_TEST_KEY",
		Models:  []string{"og-test"},
		APIBase: "/v1/og-test",
	},
}

// providerToModel returns the first model for the provider.
func providerToModel(p string) string {
	for _, provider := range ogProviders {
		if p == provider.Name {
			return provider.Models[0]
		}
	}
	return p // It must be a model already.
}

func getProviderByModel(modelName string) *OGProvider {
	for i := range ogProviders {
		for _, model := range ogProviders[i].Models {
			if modelName == model {
				return &ogProviders[i]
			}
		}
	}
	// Also check by provider name for "Continue" compatibility
	for i := range ogProviders {
		if modelName == ogProviders[i].Name {
			return &ogProviders[i]
		}
	}
	return nil
}

// Deprecated: Use getProviderByModel instead.
func getModelURLandKey(m string) (string, string) {
	provider := getProviderByModel(m)
	if provider != nil {
		return provider.URL, provider.APIKey
	}
	return "URL-Not-Found", ""
}
