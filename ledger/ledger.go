// Package ledger owns the private Workflow Ledger: the machine-local Git
// clone that records accepted Proposals, their frozen Contracts, and
// Workflow State. The public skl CLI is the only intended caller surface;
// workers never navigate this store directly.
package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Config is the single machine-local setting file. Its only setting is the
// absolute path of an existing local ledger clone.
type Config struct {
	Ledger string `json:"ledger"`
}

// SettingsLocation resolves the applicable config file: XDG_CONFIG_HOME when
// set, otherwise the user's home .config directory. The second result reports
// whether a config file exists at that location; absence is a distinct fact
// from an unusable file.
func SettingsLocation(getenv func(string) string) (string, bool, error) {
	directory := strings.TrimSpace(getenv("XDG_CONFIG_HOME"))
	if directory == "" {
		home := strings.TrimSpace(getenv("HOME"))
		if home == "" {
			return "", false, errors.New("cannot resolve the configuration location: neither XDG_CONFIG_HOME nor HOME is set; set XDG_CONFIG_HOME to a directory containing skl/config.json")
		}
		directory = filepath.Join(home, ".config")
	}
	path := filepath.Join(directory, "skl", "config.json")
	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("cannot inspect configuration %s: %w", path, err)
	}
	return path, err == nil, nil
}

// LoadConfig reads and validates the config file at path, returning a
// concrete repair message for every unusable shape. It never invents a
// default ledger location.
func LoadConfig(path string) (Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("cannot read skl configuration %s: %w; create it as {\"ledger\": \"/absolute/path/to/ledger-clone\"}", path, err)
	}
	var config Config
	decoder := json.NewDecoder(strings.NewReader(string(contents)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("malformed skl configuration %s: %v; write it as {\"ledger\": \"/absolute/path/to/ledger-clone\"}", path, err)
	}
	if strings.TrimSpace(config.Ledger) == "" {
		return Config{}, fmt.Errorf("skl configuration %s has no ledger setting; write it as {\"ledger\": \"/absolute/path/to/ledger-clone\"}", path)
	}
	if !filepath.IsAbs(config.Ledger) {
		return Config{}, fmt.Errorf("skl configuration %s has a relative ledger path %q; use an absolute path to an existing local Git clone", path, config.Ledger)
	}
	return config, nil
}

// Store is one opened local ledger clone. All mutations are brief local Git
// operations; replication happens separately through the clone's own remote
// configuration.
type Store struct {
	Root string
}

// Open validates that path is a usable local Git clone with at least one
// commit and returns the store rooted there.
func Open(path string) (*Store, error) {
	root, err := git(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("configured ledger %s is not a usable local Git clone; clone the ledger repository there and point skl/config.json at it", path)
	}
	if _, err := git(root, "rev-parse", "HEAD"); err != nil {
		return nil, fmt.Errorf("configured ledger %s has no commits; clone or initialize the ledger repository with a commit first", root)
	}
	return &Store{Root: root}, nil
}

// LoadSettings resolves, reads, and opens the applicable machine
// configuration. exists reports whether any config file was present, so
// callers can distinguish an unconfigured machine from an unusable one.
func LoadSettings(getenv func(string) string) (store *Store, path string, exists bool, err error) {
	path, exists, err = SettingsLocation(getenv)
	if err != nil || !exists {
		return nil, path, exists, err
	}
	config, err := LoadConfig(path)
	if err != nil {
		return nil, path, exists, err
	}
	store, err = Open(config.Ledger)
	if err != nil {
		return nil, path, exists, err
	}
	return store, path, true, nil
}

// Refusal is an actionable refusal: an invariant that stopped the operation
// together with the repair that would let the same operation proceed.
type Refusal struct {
	Invariant string
	Repair    string
}

func (r *Refusal) Error() string { return r.Invariant + "; " + r.Repair }

func refuse(invariant, repair string) error {
	return &Refusal{Invariant: invariant, Repair: repair}
}

// git runs one Git command in directory and returns its trimmed stdout.
func git(directory string, args ...string) (string, error) {
	output, err := exec.Command("git", append([]string{"-C", directory}, args...)...).Output()
	return strings.TrimSpace(string(output)), err
}

// gitError annotates a failed Git command with its captured stderr so
// refusals name the concrete cause.
func gitError(directory string, args []string, err error) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Errorf("git -C %s %s: %w: %s", directory, strings.Join(args, " "), err, strings.TrimSpace(string(exit.Stderr)))
	}
	return fmt.Errorf("git -C %s %s: %w", directory, strings.Join(args, " "), err)
}

func gitOK(directory string, args ...string) bool {
	return exec.Command("git", append([]string{"-C", directory}, args...)...).Run() == nil
}

// head returns the full commit ID of the ledger's current head.
func (s *Store) head() (string, error) {
	return git(s.Root, "rev-parse", "HEAD")
}
