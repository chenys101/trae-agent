// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent-go/internal/config"
)

func TestResolveTaskFromArgs(t *testing.T) {
	task, err := resolveTask([]string{"hello world"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task != "hello world" {
		t.Fatalf("expected 'hello world', got %q", task)
	}
}

func TestResolveTaskFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "task.txt")
	content := "this is a task from file"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	task, err := resolveTask(nil, tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task != content {
		t.Fatalf("expected %q, got %q", content, task)
	}
}

func TestResolveTaskEmpty(t *testing.T) {
	_, err := resolveTask(nil, "")
	if err == nil {
		t.Fatal("expected error when no task provided")
	}
}

func TestResolveTaskFileNotFound(t *testing.T) {
	_, err := resolveTask(nil, "/nonexistent/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestBuildAgentConfigDefault(t *testing.T) {
	cfg := config.DefaultConfig()

	agentCfg, err := buildAgentConfig(cfg, "", "", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agentCfg.ModelConfig.Provider != "anthropic" {
		t.Fatalf("expected provider 'anthropic', got %q", agentCfg.ModelConfig.Provider)
	}
	if agentCfg.MaxSteps != 30 {
		t.Fatalf("expected maxSteps 30, got %d", agentCfg.MaxSteps)
	}
}

func TestBuildAgentConfigWithOverrides(t *testing.T) {
	cfg := config.DefaultConfig()

	agentCfg, err := buildAgentConfig(cfg, "openai", "gpt-4o", 10, "/tmp/work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agentCfg.ModelConfig.Provider != "openai" {
		t.Fatalf("expected provider 'openai', got %q", agentCfg.ModelConfig.Provider)
	}
	if agentCfg.ModelConfig.Model != "gpt-4o" {
		t.Fatalf("expected model 'gpt-4o', got %q", agentCfg.ModelConfig.Model)
	}
	if agentCfg.MaxSteps != 10 {
		t.Fatalf("expected maxSteps 10, got %d", agentCfg.MaxSteps)
	}
	if agentCfg.WorkingDir != "/tmp/work" {
		t.Fatalf("expected workingDir '/tmp/work', got %q", agentCfg.WorkingDir)
	}
}

func TestBuildAgentConfigNoTraeAgentEntry(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "openai",
		ModelProviders: map[string]config.ModelProvider{
			"openai": {
				Provider:     "openai",
				DefaultModel: "gpt-4o",
			},
		},
		Agents: map[string]config.AgentConfig{},
	}

	agentCfg, err := buildAgentConfig(cfg, "", "", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agentCfg.ModelConfig.Provider != "openai" {
		t.Fatalf("expected provider 'openai', got %q", agentCfg.ModelConfig.Provider)
	}
	if agentCfg.MaxSteps != 30 {
		t.Fatalf("expected default maxSteps 30, got %d", agentCfg.MaxSteps)
	}
}

func TestBuildAgentConfigInvalidProvider(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "nonexistent",
		ModelProviders:  map[string]config.ModelProvider{},
		Agents:          map[string]config.AgentConfig{},
	}

	_, err := buildAgentConfig(cfg, "nonexistent", "", 0, "")
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Fatalf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestRootCmdVersion(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--version"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	_ = cmd.Execute()
	output := buf.String()
	if !strings.Contains(output, "trae version") {
		t.Fatalf("expected version output, got %q", output)
	}
}

func TestRunCmdNoArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no task provided")
	}
	if !strings.Contains(err.Error(), "任务描述") {
		t.Fatalf("expected task-related error, got %v", err)
	}
}

func TestRunCmdWithTask(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"run", "hello world", "--provider", "openai", "--model", "gpt-4o"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error due to missing API key")
	}
	if strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("unexpected flag error: %v", err)
	}
}

func TestRunCmdWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "task.txt")
	if err := os.WriteFile(tmpFile, []byte("do something"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"run", "--file", tmpFile})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error due to missing API key")
	}
	if strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("unexpected flag error: %v", err)
	}
}

func TestShowConfigCmd(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"show-config"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "default_provider") {
		t.Fatalf("expected config output to contain 'default_provider', got %q", output)
	}
}

func TestShowConfigCmdWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")
	content := "default_provider: openai\nmodel_providers:\n  openai:\n    provider: openai\n    default_model: gpt-4o\nagents:\n  trae_agent:\n    provider: openai\n    model: gpt-4o\n    max_steps: 20\n"
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"show-config", "--config", cfgFile})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "openai") {
		t.Fatalf("expected config output to contain 'openai', got %q", output)
	}
}

func TestToolsCmd(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"tools"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "bash") {
		t.Fatalf("expected tools output to contain 'bash', got %q", output)
	}
	if !strings.Contains(output, "edit") {
		t.Fatalf("expected tools output to contain 'edit', got %q", output)
	}
}

func TestRunCmdFlags(t *testing.T) {
	cmd := newRunCmd()

	flags := []string{"provider", "model", "max-steps", "working-dir", "config", "file"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Fatalf("expected flag --%s to exist", f)
		}
	}
}

func TestInteractiveCmdFlags(t *testing.T) {
	cmd := newInteractiveCmd()

	flags := []string{"provider", "model", "max-steps", "working-dir", "config"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Fatalf("expected flag --%s to exist", f)
		}
	}
}

func TestLoadConfigOrDefaultNoFile(t *testing.T) {
	cfg, err := loadConfigOrDefault("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.DefaultProvider != "anthropic" {
		t.Fatalf("expected default provider 'anthropic', got %q", cfg.DefaultProvider)
	}
}

func TestLoadConfigOrDefaultWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")
	content := "default_provider: openai\nmodel_providers:\n  openai:\n    provider: openai\n    default_model: gpt-4o\n"
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := loadConfigOrDefault(cfgFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Fatalf("expected provider 'openai', got %q", cfg.DefaultProvider)
	}
}
