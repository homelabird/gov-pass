package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	bubbleAutoRefreshInterval = 3 * time.Second
	bubbleSpinnerInterval     = 120 * time.Millisecond
)

var bubbleSpinnerFrames = []string{"-", "\\", "|", "/"}

type bubbleStatusCollector func(string) tuiStatus

type bubbleActionExecutor func(string, tuiStatus, string) (tuiFeedback, error)

type bubbleRefreshMsg struct {
	status tuiStatus
}

type bubbleActionResultMsg struct {
	status   tuiStatus
	feedback tuiFeedback
	err      error
}

type bubbleAutoRefreshMsg struct{}

type bubbleSpinnerMsg struct{}

type bubbleTUIModel struct {
	serviceName   string
	collectStatus bubbleStatusCollector
	executeAction bubbleActionExecutor

	status   tuiStatus
	feedback tuiFeedback

	width  int
	height int

	busy        bool
	busyAction  string
	spinnerTick int
}

func runBubbleTUI(serviceName string, collectStatus bubbleStatusCollector) error {
	model := newBubbleTUIModel(serviceName, collectStatus, executeMenuChoice)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := program.Run()
	return err
}

func newBubbleTUIModel(serviceName string, collectStatus bubbleStatusCollector, executeAction bubbleActionExecutor) bubbleTUIModel {
	size := currentTerminalSize()
	status := collectStatus(serviceName)
	return bubbleTUIModel{
		serviceName:   serviceName,
		collectStatus: collectStatus,
		executeAction: executeAction,
		status:        status,
		feedback:      readyTUIFeedback(),
		width:         size.Cols,
		height:        size.Rows,
	}
}

func (model bubbleTUIModel) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("gov-pass ON/OFF"),
		bubbleAutoRefreshCmd(),
	)
}

func (model bubbleTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		model.width = msg.Width
		model.height = msg.Height
		return model, nil
	case tea.KeyMsg:
		return model.updateKey(msg)
	case tea.MouseMsg:
		return model.updateMouse(msg)
	case bubbleActionResultMsg:
		model.busy = false
		model.busyAction = ""
		model.status = msg.status
		if msg.err != nil {
			model.feedback = errorTUIFeedback(msg.err)
		} else if !msg.feedback.isZero() {
			model.feedback = msg.feedback
		}
		return model, nil
	case bubbleAutoRefreshMsg:
		if model.busy {
			return model, bubbleAutoRefreshCmd()
		}
		return model, tea.Batch(
			bubbleRefreshStatusCmd(model.serviceName, model.collectStatus),
			bubbleAutoRefreshCmd(),
		)
	case bubbleRefreshMsg:
		model.status = msg.status
		return model, nil
	case bubbleSpinnerMsg:
		if !model.busy {
			return model, nil
		}
		model.spinnerTick = (model.spinnerTick + 1) % len(bubbleSpinnerFrames)
		return model, bubbleSpinnerCmd()
	}

	return model, nil
}

func (model bubbleTUIModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return model, tea.Quit
	}

	if model.busy {
		return model, nil
	}

	switch msg.String() {
	case "enter", " ":
		return model.beginToggle()
	}
	return model, nil
}

func (model bubbleTUIModel) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if model.busy {
		return model, nil
	}
	event := tea.MouseEvent(msg)
	if event.Button != tea.MouseButtonLeft || event.Action != tea.MouseActionPress {
		return model, nil
	}
	bounds := toggleButtonBoundsForSize(model.status, model.width, model.busy)
	if !bounds.contains(event.X, event.Y) {
		return model, nil
	}
	return model.beginToggle()
}

func (model bubbleTUIModel) beginToggle() (tea.Model, tea.Cmd) {
	if model.busy || serviceStateBusy(model.status.RawState) {
		return model, nil
	}
	model.busy = true
	model.busyAction = "Toggling service"
	model.spinnerTick = 0

	return model, tea.Batch(
		bubbleExecuteActionCmd(model.serviceName, model.status, "toggle", model.executeAction, model.collectStatus),
		bubbleSpinnerCmd(),
	)
}

func (model bubbleTUIModel) View() string {
	width := normalizeRenderWidth(model.width)
	if model.width <= 0 {
		width = currentTerminalSize().Cols
	}

	feedback := model.feedback
	if model.busy {
		summary := fmt.Sprintf("%s %s", bubbleSpinnerFrames[model.spinnerTick], nonEmptyString(model.busyAction, "Working"))
		feedback = infoTUIFeedback(summary, "")
	}
	return renderToggleTUIViewForSize(model.status, feedback, width, model.height, model.busy)
}

func bubbleExecuteActionCmd(serviceName string, status tuiStatus, choice string, executeAction bubbleActionExecutor, collectStatus bubbleStatusCollector) tea.Cmd {
	return func() tea.Msg {
		feedback, err := executeAction(serviceName, status, choice)
		nextStatus := collectStatus(serviceName)
		return bubbleActionResultMsg{
			status:   nextStatus,
			feedback: feedback,
			err:      err,
		}
	}
}

func bubbleRefreshStatusCmd(serviceName string, collectStatus bubbleStatusCollector) tea.Cmd {
	return func() tea.Msg {
		return bubbleRefreshMsg{status: collectStatus(serviceName)}
	}
}

func bubbleAutoRefreshCmd() tea.Cmd {
	return tea.Tick(bubbleAutoRefreshInterval, func(time.Time) tea.Msg {
		return bubbleAutoRefreshMsg{}
	})
}

func bubbleSpinnerCmd() tea.Cmd {
	return tea.Tick(bubbleSpinnerInterval, func(time.Time) tea.Msg {
		return bubbleSpinnerMsg{}
	})
}
