// Package setup contains setup discovery and transactional configuration helpers.
package setup

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"gopkg.in/yaml.v3"
)

const HistoryLimit = 5

// PreviewConfig replaces only known setup keys in the original YAML document,
// keeping unrelated configuration and keys from newer rog versions intact.
func PreviewConfig(original []byte, next *config.Config) ([]byte, error) {
	if next == nil {
		return nil, fmt.Errorf("proposed config is required")
	}
	var doc yaml.Node
	if len(bytes.TrimSpace(original)) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	} else if err := yaml.Unmarshal(original, &doc); err != nil {
		return nil, fmt.Errorf("parse existing config: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config must be a YAML mapping")
	}
	root := doc.Content[0]
	set := func(key string, value any) error {
		data, err := yaml.Marshal(value)
		if err != nil {
			return err
		}
		var node yaml.Node
		if err := yaml.Unmarshal(data, &node); err != nil {
			return err
		}
		if len(node.Content) == 0 {
			return fmt.Errorf("encode %s", key)
		}
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value == key {
				root.Content[i+1] = node.Content[0]
				return nil
			}
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, node.Content[0])
		return nil
	}
	oldRoots := mappingValue(root, "roots")
	oldReport := mappingValue(root, "report")
	if err := set("roots", next.Roots); err != nil {
		return nil, err
	}
	preserveRootExtras(oldRoots, mappingValue(root, "roots"))
	if next.GlobalExcludes != nil {
		if err := set("global_excludes", next.GlobalExcludes); err != nil {
			return nil, err
		}
	}
	if next.Report != nil {
		if err := set("report", next.Report); err != nil {
			return nil, err
		}
		preserveUnknownFields(oldReport, mappingValue(root, "report"), map[string]bool{
			"author_emails": true, "input_usd_per_million": true, "output_usd_per_million": true,
		})
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func preserveUnknownFields(old, next *yaml.Node, known map[string]bool) {
	if old == nil || next == nil || old.Kind != yaml.MappingNode || next.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(old.Content); i += 2 {
		key := old.Content[i].Value
		if !known[key] && mappingValue(next, key) == nil {
			next.Content = append(next.Content, old.Content[i], old.Content[i+1])
		}
	}
}

func preserveRootExtras(old, next *yaml.Node) {
	if old == nil || next == nil || old.Kind != yaml.SequenceNode || next.Kind != yaml.SequenceNode {
		return
	}
	known := map[string]bool{"name": true, "path": true, "max_depth": true, "exclude": true, "wsl": true, "wsl_distro": true, "windows": true}
	identity := func(node *yaml.Node) string {
		return strings.Join([]string{
			nodeValue(node, "name"), nodeValue(node, "path"), nodeValue(node, "wsl"),
			nodeValue(node, "wsl_distro"), nodeValue(node, "windows"),
		}, "\x00")
	}
	byIdentity := make(map[string]*yaml.Node, len(old.Content))
	for _, item := range old.Content {
		if item.Kind == yaml.MappingNode {
			byIdentity[identity(item)] = item
		}
	}
	for _, item := range next.Content {
		if item.Kind == yaml.MappingNode {
			preserveUnknownFields(byIdentity[identity(item)], item, known)
		}
	}
}

func nodeValue(node *yaml.Node, key string) string {
	if value := mappingValue(node, key); value != nil {
		return value.Value
	}
	return ""
}

// ValidateSetupConfig checks values the wizard owns before any config write.
func ValidateSetupConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is required")
	}
	if len(cfg.Roots) == 0 {
		return fmt.Errorf("select at least one Configured Root")
	}
	seen := make(map[string]bool, len(cfg.Roots))
	for i, root := range cfg.Roots {
		name := strings.TrimSpace(root.Name)
		if name == "" {
			return fmt.Errorf("Configured Root %d needs a name", i+1)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("Configured Root name %q is repeated", root.Name)
		}
		seen[key] = true
		if root.WSL && root.Windows {
			return fmt.Errorf("Configured Root %q cannot be both WSL and Windows", root.Name)
		}
		if strings.TrimSpace(root.Path) == "" {
			return fmt.Errorf("Configured Root %q needs a path", root.Name)
		}
		absolute := false
		switch {
		case root.Windows:
			absolute = IsWindowsDrivePath(root.Path)
		case root.WSL || runtime.GOOS != "windows":
			absolute = path.IsAbs(root.Path)
		default:
			absolute = filepath.IsAbs(root.Path)
		}
		if !absolute {
			return fmt.Errorf("Configured Root %q path must be absolute: %s", root.Name, root.Path)
		}
		if root.MaxDepth < 1 {
			return fmt.Errorf("Configured Root %q depth must be at least 1", root.Name)
		}
	}
	for _, exclude := range cfg.GlobalExcludes {
		if strings.TrimSpace(exclude) == "" {
			return fmt.Errorf("directory exclusions cannot be empty")
		}
	}
	return nil
}

// IsWindowsDrivePath reports whether value starts with an absolute Windows drive path.
// Setup uses it to classify locations entered from WSL and validate bridged roots.
func IsWindowsDrivePath(value string) bool {
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
}

// ApplyConfig writes a validated config, keeps five timestamped revisions, and
// restores the prior file if the atomic replacement or read-back fails.
func ApplyConfig(path string, data []byte, now time.Time) (string, error) {
	var check config.Config
	if err := yaml.Unmarshal(data, &check); err != nil {
		return "", fmt.Errorf("refusing invalid config: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	mode := fs.FileMode(0600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm() & 0600
		if mode == 0 {
			mode = 0600
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	history := filepath.Join(dir, "setup-history")
	if err := os.MkdirAll(history, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(history, 0700); err != nil {
		return "", fmt.Errorf("protect setup history: %w", err)
	}
	backup := ""
	committed := false
	defer func() {
		if !committed && backup != "" {
			_ = os.Remove(backup)
		}
	}()
	if prior, err := os.ReadFile(path); err == nil {
		stamp := now.Local().Format("20060102-150405.000000000")
		backupPath := filepath.Join(history, stamp+".yml")
		file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return "", fmt.Errorf("backup config: %w", err)
		}
		backup = backupPath
		if _, err := file.Write(prior); err != nil {
			_ = file.Close()
			return "", fmt.Errorf("write setup backup: %w", err)
		}
		if err := file.Close(); err != nil {
			return "", fmt.Errorf("close setup backup: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".config-setup-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("replace config: %w", err)
	}
	written, err := os.ReadFile(path)
	if err == nil {
		err = yaml.Unmarshal(written, &check)
	}
	if err != nil {
		if backup != "" {
			if restoreErr := os.Rename(backup, path); restoreErr != nil {
				committed = true // Preserve the only known-good copy in setup history.
				return "", fmt.Errorf("verify written config: %w; restoring prior config failed: %v; backup preserved at %s", err, restoreErr, backup)
			}
		} else {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				return "", fmt.Errorf("verify written config: %w; removing the unverified file failed: %v", err, removeErr)
			}
		}
		return "", fmt.Errorf("verify written config; prior config restored: %w", err)
	}
	committed = true
	// Retention runs only after the replacement is verified so a failed write
	// cannot discard the oldest usable revision. A cleanup failure keeps extra
	// protected history rather than turning an applied config into an error.
	_ = retainHistory(history, HistoryLimit)
	return backup, nil
}

func retainHistory(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yml") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if len(names) <= keep {
		return nil
	}
	for _, name := range names[keep:] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func History(path string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(path), "setup-history"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yml") {
			out = append(out, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

func RestoreConfig(path, revision string, now time.Time) (string, error) {
	if filepath.Base(revision) != revision || !strings.HasSuffix(revision, ".yml") {
		return "", fmt.Errorf("invalid setup revision %q", revision)
	}
	history := filepath.Join(filepath.Dir(path), "setup-history")
	source := filepath.Join(history, revision)
	data, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	backup, err := ApplyConfig(path, data, now)
	if err != nil {
		return "", err
	}
	return backup, nil
}
