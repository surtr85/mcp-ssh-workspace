package config

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
)

type Config struct {
	Host       string
	Port       int
	User       string
	KeyPath    string
	Password   string
	WorkDir    string
	UseAgent   bool
	KnownHosts string
}

func Parse() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.Host, "host", os.Getenv("SSH_HOST"), "Remote SSH host, IP, or ~/.ssh/config alias")
	flag.IntVar(&cfg.Port, "port", 0, "Remote SSH port (default 22 or from ssh config)")
	flag.StringVar(&cfg.User, "user", os.Getenv("SSH_USER"), "Remote SSH user")
	flag.StringVar(&cfg.KeyPath, "key", os.Getenv("SSH_KEY"), "Path to private key file (e.g. ~/.ssh/id_ed25519)")
	flag.StringVar(&cfg.Password, "password", os.Getenv("SSH_PASSWORD"), "Password for SSH authentication")
	flag.StringVar(&cfg.WorkDir, "workdir", os.Getenv("SSH_WORKDIR"), "Default initial remote working directory")
	flag.BoolVar(&cfg.UseAgent, "agent", true, "Use SSH agent ($SSH_AUTH_SOCK) if available")
	flag.StringVar(&cfg.KnownHosts, "known-hosts", "", "Path to known_hosts file")

	flag.Parse()

	if cfg.Host == "" {
		if flag.NArg() > 0 {
			arg := flag.Arg(0)
			if strings.Contains(arg, "@") {
				parts := strings.SplitN(arg, "@", 2)
				if cfg.User == "" {
					cfg.User = parts[0]
				}
				cfg.Host = parts[1]
			} else {
				cfg.Host = arg
			}
		}
	}

	if cfg.Host == "" {
		return nil, fmt.Errorf("missing remote host: specify --host, [user@]host argument, or set SSH_HOST")
	}

	homeDir, _ := os.UserHomeDir()
	sshConfigFile := filepath.Join(homeDir, ".ssh", "config")
	if f, err := os.Open(sshConfigFile); err == nil {
		defer f.Close()
		if parsedCfg, err := ssh_config.Decode(f); err == nil {
			if aliasHost, err := parsedCfg.Get(cfg.Host, "HostName"); err == nil && aliasHost != "" {
				cfg.Host = aliasHost
			}
			if cfg.User == "" {
				if user, err := parsedCfg.Get(cfg.Host, "User"); err == nil && user != "" {
					cfg.User = user
				}
			}
			if cfg.Port == 0 {
				if portStr, err := parsedCfg.Get(cfg.Host, "Port"); err == nil && portStr != "" {
					if p, err := strconv.Atoi(portStr); err == nil {
						cfg.Port = p
					}
				}
			}
			if cfg.KeyPath == "" {
				if key, err := parsedCfg.Get(cfg.Host, "IdentityFile"); err == nil && key != "" {
					if strings.HasPrefix(key, "~/") {
						key = filepath.Join(homeDir, key[2:])
					}
					cfg.KeyPath = key
				}
			}
		}
	}

	if cfg.Port == 0 {
		cfg.Port = 22
	}

	if cfg.User == "" {
		if u := os.Getenv("USER"); u != "" {
			cfg.User = u
		} else {
			cfg.User = "root"
		}
	}

	if strings.HasPrefix(cfg.KeyPath, "~/") {
		cfg.KeyPath = filepath.Join(homeDir, cfg.KeyPath[2:])
	}

	if cfg.KeyPath == "" && cfg.Password == "" && os.Getenv("SSH_AUTH_SOCK") == "" {
		defaultKeys := []string{"id_ed25519", "id_rsa", "id_ecdsa"}
		for _, dk := range defaultKeys {
			p := filepath.Join(homeDir, ".ssh", dk)
			if _, err := os.Stat(p); err == nil {
				cfg.KeyPath = p
				break
			}
		}
	}

	return cfg, nil
}

func (c *Config) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}
