package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Set writes one scalar key of a top-level mapping of the config file at path
// and keeps every other byte of the file.
//
// A YAML encoder cannot do that: it drops blank lines and re-indents what it
// decoded. So the file is edited as text, at the line and column the parser
// reports for the value. A missing key becomes a line after the last entry of
// its section, and a missing section goes at the end of the file.
//
// The new text is loaded as LoadWithOptions loads it and compared with the old
// document before anything is written: section.key must read back as value and
// every other value must read as it did. An edit that fails that, or a shape
// the text edit does not handle - a section in flow style without the key, an
// alias, a second document - is refused and the file stays as it was.
func Set(path, section, key, value string) error {
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config %q: %w", path, err)
	}
	text, err := setKey(string(old), section, key, value)
	if err == nil {
		err = checkEdit(old, []byte(text), section, key, value)
	}
	if err != nil {
		return fmt.Errorf("config %q: cannot set %s.%s: %w; change it by hand", path, section, key, err)
	}
	if err := replaceFile(path, []byte(text)); err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}
	return nil
}

// setKey returns text with section.key set to value.
func setKey(text, section, key, value string) (string, error) {
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	literal, err := scalarLiteral(value)
	if err != nil {
		return "", err
	}
	entry := key + ": " + literal

	root, err := rootNode(text)
	if err != nil {
		return "", err
	}
	if root == nil || isEmptyScalar(root) {
		return appendSection(text, "", section, entry, nl), nil
	}
	if root.Kind != yaml.MappingNode {
		return "", errors.New("the file is not a mapping")
	}

	sectionKey, sectionValue := mappingEntry(root, section)
	switch {
	case sectionKey == nil && root.Style&yaml.FlowStyle != 0:
		return "", errors.New("the file is written in flow style")
	case sectionKey == nil:
		return appendSection(text, indentOf(text, root.Content[0]), section, entry, nl), nil
	case isEmptyScalar(sectionValue):
		return insertEntry(text, sectionKey, indentOf(text, sectionKey)+"  ", entry, nl), nil
	case sectionValue.Kind != yaml.MappingNode || sectionValue.Anchor != "":
		return "", fmt.Errorf("%s is not a plain mapping", section)
	}

	_, old := mappingEntry(sectionValue, key)
	switch {
	case old == nil && sectionValue.Style&yaml.FlowStyle != 0:
		return "", fmt.Errorf("%s is written in flow style", section)
	case old == nil:
		last := sectionValue.Content[len(sectionValue.Content)-2]
		return insertEntry(text, last, indentOf(text, last), entry, nl), nil
	case old.Kind != yaml.ScalarNode || old.Anchor != "":
		return "", errors.New("its value is not a plain scalar")
	}

	start := offsetOf(text, old.Line, old.Column)
	end, ok := scalarEnd(text, start, old)
	if !ok {
		return "", errors.New("its value is not a scalar on one line")
	}
	if start > 0 && text[start-1] == ':' {
		literal = " " + literal
	}
	return text[:start] + literal + text[end:], nil
}

// rootNode is the top-level node of the one document in text, nil when text
// holds no document: an empty file, or comments only.
func rootNode(text string) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(strings.NewReader(text))
	var document yaml.Node
	if err := decoder.Decode(&document); err == io.EOF {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(yaml.Node)); err != io.EOF {
		return nil, errors.New("the file holds more than one YAML document")
	}
	return document.Content[0], nil
}

// scalarLiteral is value as YAML writes it on one line, quoted only when a
// plain scalar would read as something else.
func scalarLiteral(value string) (string, error) {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return "", err
	}
	literal := strings.TrimSuffix(string(encoded), "\n")
	if strings.Contains(literal, "\n") {
		return "", errors.New("the value does not fit on one line")
	}
	return literal, nil
}

func mappingEntry(mapping *yaml.Node, name string) (key, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			return mapping.Content[i], mapping.Content[i+1]
		}
	}
	return nil, nil
}

// isEmptyScalar reports a value with nothing written for it, as in "ui:".
func isEmptyScalar(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Value == "" && node.Style == 0
}

// offsetOf is the byte offset of a parser position. The parser counts lines and
// columns from 1, and a column in characters.
func offsetOf(text string, line, column int) int {
	offset := 0
	for ; line > 1; line-- {
		offset = lineEnd(text, offset)
	}
	for ; column > 1; column-- {
		_, size := utf8.DecodeRuneInString(text[offset:])
		offset += size
	}
	return offset
}

// lineEnd is the offset after the line break of the line that holds offset.
func lineEnd(text string, offset int) int {
	i := strings.IndexByte(text[offset:], '\n')
	if i < 0 {
		return len(text)
	}
	return offset + i + 1
}

// indentOf is what stands before node on its line.
func indentOf(text string, node *yaml.Node) string {
	return text[offsetOf(text, node.Line, 1):offsetOf(text, node.Line, node.Column)]
}

// scalarEnd finds where the scalar that starts at text[start] ends. A quoted
// scalar is longer in the file than its value, so it is scanned to its closing
// quote; a plain one is its value as written.
func scalarEnd(text string, start int, scalar *yaml.Node) (int, bool) {
	switch scalar.Style {
	case 0:
		return start + len(scalar.Value), strings.HasPrefix(text[start:], scalar.Value)
	case yaml.DoubleQuotedStyle:
		for i := start + 1; i < len(text); i++ {
			switch text[i] {
			case '\\':
				i++
			case '"':
				return i + 1, text[start] == '"'
			}
		}
	case yaml.SingleQuotedStyle:
		for i := start + 1; i < len(text); i++ {
			if text[i] != '\'' {
				continue
			}
			if strings.HasPrefix(text[i+1:], "'") {
				i++
				continue
			}
			return i + 1, text[start] == '\''
		}
	}
	return 0, false
}

// insertEntry puts entry on a line of its own after the mapping entry that
// starts at the key after: past the lines of its value, which are indented
// deeper than the key, and before the blank lines and comments that follow.
func insertEntry(text string, after *yaml.Node, indent, entry, nl string) string {
	at := lineEnd(text, offsetOf(text, after.Line, 1))
	for next := at; next < len(text); {
		end := lineEnd(text, next)
		body := strings.TrimLeft(text[next:end], " \t")
		if content := strings.TrimRight(body, "\r\n"); content != "" && content[0] != '#' {
			if end-next-len(body) < after.Column {
				break
			}
			at = end
		}
		next = end
	}
	if at == len(text) && !strings.HasSuffix(text, "\n") {
		text += nl
		at = len(text)
	}
	return text[:at] + indent + entry + nl + text[at:]
}

// appendSection adds the section, with its one entry, at the end of the file.
func appendSection(text, indent, section, entry, nl string) string {
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += nl
	}
	if text != "" && !strings.HasSuffix(text, nl+nl) {
		text += nl
	}
	return text + indent + section + ":" + nl + indent + "  " + entry + nl
}

// checkEdit refuses an edit the loader would not read as intended: the new
// text must load, and must decode to the old document with section.key set to
// value and nothing else changed.
func checkEdit(old, edited []byte, section, key, value string) error {
	override, _, err := decodeOverride(edited)
	if err != nil {
		return err
	}
	if _, err := validateResolved(merge(Default(), override)); err != nil {
		return err
	}

	var want, got map[string]any
	if err := yaml.Unmarshal(old, &want); err != nil {
		return err
	}
	if err := yaml.Unmarshal(edited, &got); err != nil {
		return err
	}
	if want == nil {
		want = map[string]any{}
	}
	entries, ok := want[section].(map[string]any)
	if !ok {
		entries = map[string]any{}
		want[section] = entries
	}
	entries[key] = value
	if !reflect.DeepEqual(got, want) {
		return errors.New("the edit would not read back as written")
	}
	return nil
}

// replaceFile writes a file whole or not at all: a crash halfway through
// leaves the old file, not half of the new one. A symlink is written through,
// so a config file linked in from a dotfiles repository stays linked, and an
// existing file keeps its mode. The data is synced before the rename, so a
// power loss does not leave the new name on an empty file.
func replaceFile(path string, data []byte) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	// A rename asks the directory, not the file, so a file the operator made
	// read-only is asked here.
	if file, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
		_ = file.Close()
	} else if !os.IsNotExist(err) {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, err = temp.Write(data)
	if err == nil {
		err = temp.Chmod(mode)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(temp.Name())
	}
	return err
}
