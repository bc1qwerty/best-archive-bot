package config

import (
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Settings holds all configuration values.
var (
	BotToken string
	ChatID   string
	AdminID  int64

	MaxConcurrentRequests = 4
	RequestDelayMin       = 1.0
	RequestDelayMax       = 3.0
	MaxPostsPerCommunity  = 20
	MinVotes              = 100
	MinViews              = 20000
	MinComments           = 150

	DBPath            string
	RecordExpireHours = 24 * 7 // 7 days
)

// Community represents a single community entry from communities.yaml.
type Community struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Enabled  bool   `yaml:"enabled"`
	Encoding string `yaml:"encoding,omitempty"`
}

// CommunitiesConfig is the top-level YAML structure.
type CommunitiesConfig struct {
	Communities map[string]Community `yaml:"communities"`
}

// Communities holds parsed community data, keyed by community ID.
var Communities map[string]Community

// Load reads .env and communities.yaml from the project root directory.
func Load(baseDir string) {
	// Load .env (ignore error if not present)
	_ = godotenv.Load(filepath.Join(baseDir, ".env"))

	BotToken = os.Getenv("BOT_TOKEN")
	ChatID = os.Getenv("CHAT_ID")
	if v := os.Getenv("ADMIN_ID"); v != "" {
		AdminID, _ = strconv.ParseInt(v, 10, 64)
	}

	DBPath = filepath.Join(baseDir, "data", "posts.db")

	// Load communities.yaml
	yamlPath := filepath.Join(baseDir, "config", "communities.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		log.Fatalf("Failed to read communities.yaml: %v", err)
	}

	var cfg CommunitiesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse communities.yaml: %v", err)
	}
	Communities = cfg.Communities
}
