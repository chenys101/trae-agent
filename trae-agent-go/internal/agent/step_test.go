// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package agent

import "testing"

func TestNewAgentExecution(t *testing.T) {
	exec := NewAgentExecution("fix the bug")

	if exec.Task != "fix the bug" {
		t.Errorf("expected task 'fix the bug', got '%s'", exec.Task)
	}
	if exec.AgentState != StateIdle {
		t.Errorf("expected state idle, got '%s'", exec.AgentState)
	}
	if exec.Success {
		t.Error("expected success to be false initially")
	}
	if len(exec.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(exec.Steps))
	}
}

func TestAgentExecutionAddStep(t *testing.T) {
	exec := NewAgentExecution("test task")

	step := AgentStep{
		StepNumber: 1,
		State:      StepThinking,
	}
	exec.AddStep(step)

	if len(exec.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(exec.Steps))
	}
	if exec.Steps[0].StepNumber != 1 {
		t.Errorf("expected step number 1, got %d", exec.Steps[0].StepNumber)
	}
}

func TestAgentExecutionAddTokenUsage(t *testing.T) {
	exec := NewAgentExecution("test task")

	exec.AddTokenUsage(TokenUsage{InputTokens: 100, OutputTokens: 50})
	if exec.TotalTokens == nil {
		t.Fatal("expected total tokens to be set")
	}
	if exec.TotalTokens.InputTokens != 100 {
		t.Errorf("expected input tokens 100, got %d", exec.TotalTokens.InputTokens)
	}

	exec.AddTokenUsage(TokenUsage{InputTokens: 200, OutputTokens: 100})
	if exec.TotalTokens.InputTokens != 300 {
		t.Errorf("expected cumulative input tokens 300, got %d", exec.TotalTokens.InputTokens)
	}
	if exec.TotalTokens.OutputTokens != 150 {
		t.Errorf("expected cumulative output tokens 150, got %d", exec.TotalTokens.OutputTokens)
	}
}

func TestAgentStepStates(t *testing.T) {
	states := map[AgentStepState]string{
		StepThinking:    "thinking",
		StepCallingTool: "calling_tool",
		StepReflecting:  "reflecting",
		StepCompleted:   "completed",
		StepError:       "error",
	}
	for state, expected := range states {
		if string(state) != expected {
			t.Errorf("expected state '%s', got '%s'", expected, state)
		}
	}
}

func TestAgentStates(t *testing.T) {
	states := map[AgentState]string{
		StateIdle:      "idle",
		StateRunning:   "running",
		StateCompleted: "completed",
		StateError:     "error",
	}
	for state, expected := range states {
		if string(state) != expected {
			t.Errorf("expected state '%s', got '%s'", expected, state)
		}
	}
}
