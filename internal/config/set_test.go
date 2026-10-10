package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetEditsOnlyTheValueItWasAskedFor(t *testing.T) {
	tests := []struct {
		name    string
		before  string
		section string
		key     string
		value   string
		after   string
	}{
		{
			name: "an existing value is replaced and every comment, blank line and space stays",
			before: "# my config\n\neditor:\n  command: nvim   # trailing\n\n" +
				"ui:\n    # the light one\n    theme:   catppuccin-mocha   # dark\n    glyphs: unicode\n\n" +
				"keybindings:\n  shell:\n    quit: [ctrl+q]\n",
			section: "ui", key: "theme", value: "catppuccin-latte",
			after: "# my config\n\neditor:\n  command: nvim   # trailing\n\n" +
				"ui:\n    # the light one\n    theme:   catppuccin-latte   # dark\n    glyphs: unicode\n\n" +
				"keybindings:\n  shell:\n    quit: [ctrl+q]\n",
		},
		{
			name:    "a double-quoted value is replaced up to its closing quote",
			before:  "ui:\n  theme: \"catppuccin-\\\"mocha\" # dark\n  glyphs: unicode\n",
			section: "ui", key: "theme", value: "catppuccin-latte",
			after: "ui:\n  theme: catppuccin-latte # dark\n  glyphs: unicode\n",
		},
		{
			name:    "a single-quoted value is replaced up to its closing quote",
			before:  "ui:\n  glyphs: 'uni''code' # letters\n",
			section: "ui", key: "glyphs", value: "ascii",
			after: "ui:\n  glyphs: ascii # letters\n",
		},
		{
			name:    "a key of a flow-style section is replaced in place",
			before:  "ui: {theme: catppuccin-mocha, glyphs: unicode}\n",
			section: "ui", key: "glyphs", value: "nerd",
			after: "ui: {theme: catppuccin-mocha, glyphs: nerd}\n",
		},
		{
			name:    "a key with no value gets one",
			before:  "ui:\n  theme:\n  glyphs: unicode\n",
			section: "ui", key: "theme", value: "catppuccin-frappe",
			after: "ui:\n  theme: catppuccin-frappe\n  glyphs: unicode\n",
		},
		{
			name:    "a key with no value keeps the comment on its line",
			before:  "ui:\n  theme:   # not chosen yet\n",
			section: "ui", key: "theme", value: "catppuccin-frappe",
			after: "ui:\n  theme: catppuccin-frappe   # not chosen yet\n",
		},
		{
			name:    "a file with CRLF line ends keeps them",
			before:  "ui:\r\n  theme: catppuccin-mocha\r\neditor:\r\n  command: vi\r\n",
			section: "ui", key: "glyphs", value: "ascii",
			after: "ui:\r\n  theme: catppuccin-mocha\r\n  glyphs: ascii\r\neditor:\r\n  command: vi\r\n",
		},
		{
			name:    "a value at the end of a file with no last line break is replaced",
			before:  "ui:\n  theme: catppuccin-mocha",
			section: "ui", key: "theme", value: "catppuccin-latte",
			after: "ui:\n  theme: catppuccin-latte",
		},
		{
			name:    "a value after characters wider than one byte is found by its column",
			before:  "ui: {note: \"é›‹\", theme: catppuccin-mocha}\n",
			section: "ui", key: "theme", value: "catppuccin-latte",
			after: "ui: {note: \"é›‹\", theme: catppuccin-latte}\n",
		},
		{
			name:    "a value YAML would read as something else is quoted",
			before:  "editor:\n  command: vi\n",
			section: "editor", key: "command", value: "true",
			after: "editor:\n  command: \"true\"\n",
		},
		{
			name: "a missing key goes after the last entry of its section, indented as its siblings",
			before: "ui:\n    theme: catppuccin-mocha # dark\n\n    # more to come\n\n" +
				"editor:\n  command: vi\n",
			section: "ui", key: "glyphs", value: "ascii",
			after: "ui:\n    theme: catppuccin-mocha # dark\n    glyphs: ascii\n\n    # more to come\n\n" +
				"editor:\n  command: vi\n",
		},
		{
			name: "a missing key goes after a last entry that spans several lines",
			before: "keybindings:\n  shell:\n    quit:\n      - ctrl+q\n\n      # and\n      - ctrl+c\n" +
				"ui:\n  theme: catppuccin-mocha\n",
			section: "keybindings", key: "note", value: "mine",
			after: "keybindings:\n  shell:\n    quit:\n      - ctrl+q\n\n      # and\n      - ctrl+c\n  note: mine\n" +
				"ui:\n  theme: catppuccin-mocha\n",
		},
		{
			name:    "a missing key goes on a new line when the file has no last line break",
			before:  "ui:\n  theme: catppuccin-mocha",
			section: "ui", key: "glyphs", value: "nerd",
			after: "ui:\n  theme: catppuccin-mocha\n  glyphs: nerd\n",
		},
		{
			name:    "a section with no value gets its first entry",
			before:  "ui: # appearance\neditor:\n  command: vi\n",
			section: "ui", key: "theme", value: "catppuccin-latte",
			after: "ui: # appearance\n  theme: catppuccin-latte\neditor:\n  command: vi\n",
		},
		{
			name:    "a missing section is added at the end of the file",
			before:  "# mine\neditor:\n  command: vi # the old one\n",
			section: "ui", key: "theme", value: "catppuccin-latte",
			after: "# mine\neditor:\n  command: vi # the old one\n\nui:\n  theme: catppuccin-latte\n",
		},
		{
			name:    "a missing section is added to a file with no last line break",
			before:  "editor:\r\n  command: vi",
			section: "ui", key: "glyphs", value: "ascii",
			after: "editor:\r\n  command: vi\r\n\r\nui:\r\n  glyphs: ascii\r\n",
		},
		{
			name:    "an empty file gets the section",
			before:  "",
			section: "ui", key: "theme", value: "catppuccin-latte",
			after: "ui:\n  theme: catppuccin-latte\n",
		},
		{
			name:    "a file of comments keeps them and gets the section",
			before:  "# nothing set yet\n",
			section: "ui", key: "theme", value: "catppuccin-latte",
			after: "# nothing set yet\n\nui:\n  theme: catppuccin-latte\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, path, tt.before)

			if err := Set(path, tt.section, tt.key, tt.value); err != nil {
				t.Fatalf("Set returned error: %v", err)
			}

			if got := readFile(t, path); got != tt.after {
				t.Fatalf("file after Set:\n%q\nwant:\n%q", got, tt.after)
			}
		})
	}
}

func TestSetCreatesAMissingFileAndItsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taskmgr-ui", "config.yaml")

	if err := Set(path, "ui", "glyphs", "ascii"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	if got, want := readFile(t, path), "ui:\n  glyphs: ascii\n"; got != want {
		t.Fatalf("file after Set: %q, want %q", got, want)
	}
	assertMode(t, path, 0o644)
	assertMode(t, filepath.Dir(path), 0o755)
}

func TestSetWritesWhatLoadReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "editor:\n  command: nvim\n")

	if err := Set(path, "ui", "theme", "catppuccin-latte"); err != nil {
		t.Fatalf("Set theme returned error: %v", err)
	}
	if err := Set(path, "ui", "glyphs", "ascii"); err != nil {
		t.Fatalf("Set glyphs returned error: %v", err)
	}

	result, err := LoadWithOptions(LoadOptions{Path: path, RequireExplicit: true})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}
	if result.Config.UI.Theme != "catppuccin-latte" || result.Config.UI.Glyphs != "ascii" {
		t.Fatalf("loaded theme=%q glyphs=%q", result.Config.UI.Theme, result.Config.UI.Glyphs)
	}
	if result.Config.Editor.Command != "nvim" {
		t.Fatalf("the editor command changed to %q", result.Config.Editor.Command)
	}
}

func TestSetRefusesWhatItCannotEditSafelyAndLeavesTheFile(t *testing.T) {
	tests := []struct {
		name    string
		before  string
		section string
		key     string
		value   string
	}{
		{name: "the section is a scalar", before: "ui: dark\n", section: "ui", key: "theme", value: "x"},
		{name: "the section is a sequence", before: "ui:\n  - theme\n", section: "ui", key: "theme", value: "x"},
		{name: "the section is written as null", before: "ui: ~\n", section: "ui", key: "theme", value: "x"},
		{name: "the file is not a mapping", before: "- ui\n", section: "ui", key: "theme", value: "x"},
		{name: "the key is missing in a flow-style section", before: "ui: {theme: catppuccin-mocha}\n", section: "ui", key: "glyphs", value: "ascii"},
		{name: "the section is missing in a flow-style file", before: "{editor: {command: vi}}\n", section: "ui", key: "theme", value: "x"},
		{name: "the file holds two documents", before: "ui:\n  theme: a\n---\nui:\n  theme: b\n", section: "ui", key: "theme", value: "x"},
		{name: "the file ends its document", before: "editor:\n  command: vi\n...\n", section: "ui", key: "theme", value: "x"},
		{name: "the section is an alias", before: "base: &base\n  theme: a\nui: *base\n", section: "ui", key: "theme", value: "x"},
		{name: "the section carries an anchor", before: "ui: &look\n  theme: a\nother: *look\n", section: "ui", key: "theme", value: "x"},
		{name: "the value is an alias", before: "base: &name a\nui:\n  theme: *name\n", section: "ui", key: "theme", value: "x"},
		{name: "the value carries an anchor", before: "ui:\n  theme: &name a\n  glyphs: *name\n", section: "ui", key: "theme", value: "x"},
		{name: "the value carries a tag", before: "ui:\n  theme: !!str a\n", section: "ui", key: "theme", value: "x"},
		{name: "the value is a block scalar", before: "ui:\n  theme: |\n    a\n", section: "ui", key: "theme", value: "x"},
		{name: "the value is a mapping", before: "ui:\n  theme:\n    name: a\n", section: "ui", key: "theme", value: "x"},
		{name: "the value runs over two lines", before: "ui:\n  theme: a\n    b\n", section: "ui", key: "theme", value: "x"},
		{name: "the new value does not fit on one line", before: "ui:\n  theme: a\n", section: "ui", key: "theme", value: "a\nb"},
		{name: "the key is written twice", before: "ui:\n  theme: a\n  theme: b\n", section: "ui", key: "theme", value: "x"},
		{name: "the file is not YAML", before: "ui: [theme\n", section: "ui", key: "theme", value: "x"},
		{name: "the loader refuses the new value", before: "ui:\n  show_mode_switcher_help: true\n", section: "ui", key: "show_mode_switcher_help", value: "maybe"},
		{name: "the entry after the last one sits on the key's column", before: "launcher:\n  definitions:\n  - action: nvim\n    command: nvim\n", section: "launcher", key: "note", value: "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, path, tt.before)

			err := Set(path, tt.section, tt.key, tt.value)

			if err == nil {
				t.Fatalf("Set returned no error; the file is now %q", readFile(t, path))
			}
			name := tt.section + "." + tt.key
			if !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "change it by hand") {
				t.Fatalf("the error must name %s and say to change it by hand, got %q", name, err)
			}
			if got := readFile(t, path); got != tt.before {
				t.Fatalf("a refused Set changed the file to %q", got)
			}
			if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
				t.Fatalf("a refused Set left %d entries in the directory, want the config file only", len(entries))
			}
		})
	}
}

func TestSetWritesThroughASymlinkAndKeepsTheMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(target, []byte("ui:\n  theme: catppuccin-mocha\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	link := filepath.Join(dir, "config.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}

	if err := Set(link, "ui", "theme", "catppuccin-latte"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the config path is no longer a symlink: %v, %v", info, err)
	}
	if got, want := readFile(t, target), "ui:\n  theme: catppuccin-latte\n"; got != want {
		t.Fatalf("the link target holds %q, want %q", got, want)
	}
	assertMode(t, target, 0o600)
	if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
		t.Fatalf("Set left %d entries beside the target, want the config file only", len(entries))
	}
}

// The rename needs the directory only, so the write bits of the file are
// asked for themselves.
func TestSetRefusesAFileTheOperatorMadeReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	before := "ui:\n  theme: catppuccin-mocha\n"
	if err := os.WriteFile(path, []byte(before), 0o444); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	err := Set(path, "ui", "theme", "catppuccin-latte")

	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected a permission error for a read-only file, got %v", err)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("the read-only file holds %q, want %q", got, before)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("Set left %d entries beside the file, want the config file only", len(entries))
	}
}

func TestSetReportsAPathItCannotRead(t *testing.T) {
	dir := t.TempDir()

	err := Set(dir, "ui", "theme", "catppuccin-latte")

	if err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected a read error for a directory, got %v", err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	return string(data)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s has mode %v, want %v", path, got, want)
	}
}
