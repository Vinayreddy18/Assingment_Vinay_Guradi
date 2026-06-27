// Package config loads runtime configuration from the environment and selects
// the language-model provider. The guiding rule: the service must boot and run
// with zero configuration (falling back to the offline provider), and light up
// the real model the moment an API key is present.
package config

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/pranav/samadhan/internal/llm"
)

// Config holds all runtime settings.
type Config struct {
	Addr            string
	WebDir          string
	AnthropicAPIKey string
	AnthropicModel  string
	OpenAIAPIKey    string
	OpenAIModel     string
	ForceProvider   string // "", "anthropic", "openai", or "mock"
	MaxRounds       int
	Seed            bool
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return Config{
		Addr:            env("SAMADHAN_ADDR", ":8080"),
		WebDir:          env("SAMADHAN_WEB_DIR", "web"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:  env("SAMADHAN_MODEL", "claude-sonnet-4-6"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:     env("SAMADHAN_OPENAI_MODEL", "gpt-4.1-mini"),
		ForceProvider:   os.Getenv("SAMADHAN_PROVIDER"),
		MaxRounds:       envInt("SAMADHAN_MAX_ROUNDS", 3),
		Seed:            envBool("SAMADHAN_SEED", true),
	}
}

// BuildProvider chooses the model provider based on configuration and reports
// the choice. With a live-provider key it uses that provider; otherwise it uses
// the deterministic offline provider so the system is always runnable.
func (c Config) BuildProvider(log *slog.Logger) llm.Provider {
	switch c.ForceProvider {
	case "mock":
		log.Info("language model: offline deterministic provider (forced)")
		return llm.NewMock()
	case "anthropic":
		if c.AnthropicAPIKey == "" {
			log.Warn("SAMADHAN_PROVIDER=anthropic but ANTHROPIC_API_KEY is empty; using offline provider")
			return llm.NewMock()
		}
		return c.buildAnthropic(log)
	case "openai":
		if c.OpenAIAPIKey == "" {
			log.Warn("SAMADHAN_PROVIDER=openai but OPENAI_API_KEY is empty; using offline provider")
			return llm.NewMock()
		}
		return c.buildOpenAI(log)
	}

	if c.AnthropicAPIKey != "" {
		return c.buildAnthropic(log)
	}
	if c.OpenAIAPIKey != "" {
		return c.buildOpenAI(log)
	}
	log.Info("language model: offline deterministic provider (set ANTHROPIC_API_KEY or OPENAI_API_KEY to use a live model)")
	return llm.NewMock()
}

func (c Config) buildAnthropic(log *slog.Logger) llm.Provider {
	if c.AnthropicAPIKey == "" {
		log.Warn("SAMADHAN_PROVIDER=anthropic but ANTHROPIC_API_KEY is empty; using offline provider")
		return llm.NewMock()
	}
	log.Info("language model: Anthropic", "model", c.AnthropicModel)
	return llm.NewAnthropic(llm.AnthropicConfig{
		APIKey: c.AnthropicAPIKey,
		Model:  c.AnthropicModel,
	})
}

func (c Config) buildOpenAI(log *slog.Logger) llm.Provider {
	log.Info("language model: OpenAI", "model", c.OpenAIModel)
	return llm.NewOpenAI(llm.OpenAIConfig{
		APIKey: c.OpenAIAPIKey,
		Model:  c.OpenAIModel,
	})
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
