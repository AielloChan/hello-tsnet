package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const fileVersion = 1

type Store struct {
	path string
}

type fileData struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() ([]Rule, error) {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Rule{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open rules: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var data fileData
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("decode rules: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if data.Version != fileVersion {
		return nil, fmt.Errorf("unsupported rules version %d", data.Version)
	}
	if err := ValidateRules(data.Rules); err != nil {
		return nil, fmt.Errorf("validate rules: %w", err)
	}
	return append([]Rule(nil), data.Rules...), nil
}

func (s *Store) Save(rules []Rule) error {
	if err := ValidateRules(rules); err != nil {
		return fmt.Errorf("validate rules: %w", err)
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".rules-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary rules file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	closeWithError := func() {
		_ = tmp.Close()
	}
	if err := tmp.Chmod(0o600); err != nil {
		closeWithError()
		return fmt.Errorf("set temporary rules permissions: %w", err)
	}
	data := fileData{Version: fileVersion, Rules: append([]Rule(nil), rules...)}
	if data.Rules == nil {
		data.Rules = []Rule{}
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		closeWithError()
		return fmt.Errorf("encode rules: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		closeWithError()
		return fmt.Errorf("sync temporary rules file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary rules file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace rules file: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("set rules permissions: %w", err)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing rules data: %w", err)
	}
	return fmt.Errorf("rules file contains multiple JSON values")
}
