package config

import "flag"

// ParseConfigPath accepts both direct Go flags and package-manager pass-through
// forms such as `pnpm dev:control-plane -- --config config/config.yaml`.
func ParseConfigPath(args []string) (string, error) {
	normalized := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--" {
			continue
		}
		normalized = append(normalized, arg)
	}

	flags := flag.NewFlagSet("control-plane", flag.ContinueOnError)
	configPath := flags.String("config", "", "path to control-plane YAML config file")
	if err := flags.Parse(normalized); err != nil {
		return "", err
	}

	return *configPath, nil
}
