package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBubbleTUIModelExecutesToggle(t *testing.T) {
	initial := newTUIStatus(tuiStatusInput{
		Platform:    "Linux",
		ServiceName: "gov-pass",
		RawState:    "active",
		Active:      true,
		ActiveKnown: true,
	})
	updatedStatus := newTUIStatus(tuiStatusInput{
		Platform:    "Linux",
		ServiceName: "gov-pass",
		RawState:    "stopped",
		ActiveKnown: true,
	})

	var gotChoice string
	model := bubbleTUIModel{
		serviceName:   "gov-pass",
		collectStatus: func(string) tuiStatus { return updatedStatus },
		executeAction: func(service string, status tuiStatus, choice string) (tuiFeedback, error) {
			gotChoice = choice
			return successTUIFeedback("Service stop requested.", "Target service: "+service), nil
		},
		status:   initial,
		feedback: readyTUIFeedback(),
	}

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	busy := next.(bubbleTUIModel)
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
	if gotChoice != "toggle" {
		t.Fatalf("executed choice = %q, want toggle", gotChoice)
	}
	if done.status.RawState != "stopped" {
		t.Fatalf("updated status = %q, want stopped", done.status.RawState)
	}
}

func TestBubbleTUIModelMouseClickRunsToggle(t *testing.T) {
	status := newTUIStatus(tuiStatusInput{
		Platform:    "Windows",
		ServiceName: "gov-pass",
		RawState:    "stopped",
		ActiveKnown: true,
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

	bounds := toggleButtonBoundsForSize(status, model.width, false)
	next, cmd := model.Update(tea.MouseMsg(tea.MouseEvent{
		X:      bounds.left,
		Y:      bounds.row,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}))
	updated := next.(bubbleTUIModel)
	if cmd == nil || !updated.busy {
		t.Fatal("left click on the button must dispatch toggle")
	}

	next, cmd = model.Update(tea.MouseMsg(tea.MouseEvent{
		X:      bounds.left,
		Y:      bounds.row - 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}))
	updated = next.(bubbleTUIModel)
	if cmd != nil || updated.busy {
		t.Fatal("click outside the button must be ignored")
	}
}

func TestBubbleTUIModelBusyStateDisablesToggle(t *testing.T) {
	status := newTUIStatus(tuiStatusInput{
		Platform:    "Windows",
		ServiceName: "gov-pass",
		RawState:    "start-pending",
		ActiveKnown: true,
	})
	model := bubbleTUIModel{status: status, feedback: readyTUIFeedback(), width: 64, height: 20}

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(bubbleTUIModel)
	if cmd != nil || updated.busy {
		t.Fatal("busy service must not dispatch another toggle")
	}
}

func TestBubbleTUIViewIsOneButton(t *testing.T) {
	linuxStatus := newTUIStatus(tuiStatusInput{
		Platform:    "Linux",
		ServiceName: "gov-pass",
		RawState:    "active",
		Active:      true,
		ActiveKnown: true,
	})
	model := bubbleTUIModel{
		status:   linuxStatus,
		feedback: readyTUIFeedback(),
		width:    64,
		height:   20,
	}

	view := model.View()
	for _, want := range []string{"STATUS: ON", "[ TURN OFF ]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("single-button view missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Restart service", "Toggle boot", "Reload service", "ACTIONS"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("single-button view unexpectedly contains %q:\n%s", unwanted, view)
		}
	}
}
