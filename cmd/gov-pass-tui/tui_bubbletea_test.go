package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBubbleTUIModelSelectionWraps(t *testing.T) {
	status := newTUIStatus(tuiStatusInput{
		Platform:     "Linux",
		ServiceName:  "gov-pass",
		RawState:     "active",
		Active:       true,
		ActiveKnown:  true,
		Enabled:      true,
		EnabledKnown: true,
		Capabilities: tuiCapabilities{Reload: true},
	})

	model := bubbleTUIModel{
		serviceName:   "gov-pass",
		collectStatus: func(string) tuiStatus { return status },
		executeAction: func(string, tuiStatus, string) (tuiFeedback, error) { return tuiFeedback{}, nil },
		status:        status,
		feedback:      readyTUIFeedback(),
		width:         100,
		height:        30,
	}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated := next.(bubbleTUIModel)
	selected := selectedAction(tuiActionsForStatus(updated.status), updated.selected)
	if selected.ID != tuiActionQuit {
		t.Fatalf("wrapped selection = %q, want quit", selected.ID)
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated = next.(bubbleTUIModel)
	selected = selectedAction(tuiActionsForStatus(updated.status), updated.selected)
	if selected.ID != tuiActionToggleService {
		t.Fatalf("selection after wrap = %q, want toggle-service", selected.ID)
	}
}

func TestBubbleTUIModelExecutesSelectedAction(t *testing.T) {
	initial := newTUIStatus(tuiStatusInput{
		Platform:     "Linux",
		ServiceName:  "gov-pass",
		RawState:     "active",
		Active:       true,
		ActiveKnown:  true,
		Enabled:      true,
		EnabledKnown: true,
		Capabilities: tuiCapabilities{Reload: true},
	})
	updatedStatus := newTUIStatus(tuiStatusInput{
		Platform:     "Linux",
		ServiceName:  "gov-pass",
		RawState:     "reloading",
		Active:       false,
		ActiveKnown:  true,
		Enabled:      true,
		EnabledKnown: true,
		Capabilities: tuiCapabilities{Reload: true},
	})

	var gotChoice string
	model := bubbleTUIModel{
		serviceName:   "gov-pass",
		collectStatus: func(string) tuiStatus { return updatedStatus },
		executeAction: func(service string, status tuiStatus, choice string) (tuiFeedback, error) {
			gotChoice = choice
			return successTUIFeedback("Service restart requested.", "Target service: "+service, false), nil
		},
		status:   initial,
		feedback: readyTUIFeedback(),
		width:    100,
		height:   30,
		selected: 1,
	}

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	busy := next.(bubbleTUIModel)
	if !busy.busy {
		t.Fatal("model should be busy after starting an action")
	}
	if gotChoice != "" {
		t.Fatalf("action executed too early: %q", gotChoice)
	}

	message := cmd()
	batch, ok := message.(tea.BatchMsg)
	if !ok {
		t.Fatalf("command message type = %T, want tea.BatchMsg", message)
	}

	var actionMsg tea.Msg
	for _, queued := range batch {
		msg := queued()
		if _, ok := msg.(bubbleSpinnerMsg); ok {
			continue
		}
		actionMsg = msg
	}
	if actionMsg == nil {
		t.Fatal("expected an action result message")
	}

	next, _ = busy.Update(actionMsg)
	done := next.(bubbleTUIModel)
	if done.busy {
		t.Fatal("model should stop being busy after action result")
	}
	if gotChoice != "2" {
		t.Fatalf("executed choice = %q, want 2", gotChoice)
	}
	if done.status.RawState != "reloading" {
		t.Fatalf("updated status = %q, want reloading", done.status.RawState)
	}
	if done.feedback.Summary != "Service restart requested." {
		t.Fatalf("feedback summary = %q", done.feedback.Summary)
	}
}

func TestBubbleTUIModelUnsupportedReloadShowsError(t *testing.T) {
	status := newTUIStatus(tuiStatusInput{
		Platform:     "FreeBSD",
		ServiceName:  "gov-pass",
		RawState:     "active",
		Active:       true,
		ActiveKnown:  true,
		Enabled:      true,
		EnabledKnown: true,
		Capabilities: tuiCapabilities{Reload: false},
	})

	model := bubbleTUIModel{
		serviceName:   "gov-pass",
		collectStatus: func(string) tuiStatus { return status },
		executeAction: func(string, tuiStatus, string) (tuiFeedback, error) { return tuiFeedback{}, nil },
		status:        status,
		feedback:      readyTUIFeedback(),
		width:         100,
		height:        30,
	}

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	updated := next.(bubbleTUIModel)
	if cmd != nil {
		t.Fatal("unsupported reload should not dispatch a command")
	}
	if updated.feedback.Level != tuiFeedbackError {
		t.Fatalf("feedback level = %q, want ERROR", updated.feedback.Level)
	}
}
