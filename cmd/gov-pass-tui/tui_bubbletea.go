package main

import (
	"fmt"
	"strings"
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

	selected    int
	busy        bool
	busyAction  string
	spinnerTick int
}

func runBubbleTUI(serviceName string, collectStatus bubbleStatusCollector) error {
	model := newBubbleTUIModel(serviceName, collectStatus, executeMenuChoice)
	program := tea.NewProgram(model, tea.WithAltScreen())
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
		tea.SetWindowTitle("gov-pass operator panel"),
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
	case bubbleActionResultMsg:
		model.busy = false
		model.busyAction = ""
		model.status = msg.status
		model.selected = clampSelectedAction(model.selected, tuiActionsForStatus(model.status))
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
		model.selected = clampSelectedAction(model.selected, tuiActionsForStatus(model.status))
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

	actions := tuiActionsForStatus(model.status)

	switch msg.String() {
	case "up", "k", "shift+tab":
		model.selected = moveSelectedAction(model.selected, -1, actions)
		return model, nil
	case "down", "j", "tab":
		model.selected = moveSelectedAction(model.selected, 1, actions)
		return model, nil
	case "enter", " ":
		if len(actions) == 0 {
			return model, nil
		}
		return model.beginAction(actions[model.selected].Key)
	default:
		if !isDirectActionShortcut(model.status, msg.String()) {
			return model, nil
		}
		return model.beginAction(msg.String())
	}
}

func (model bubbleTUIModel) beginAction(choice string) (tea.Model, tea.Cmd) {
	action, err := resolveTUIAction(model.status, choice)
	if err != nil {
		if choice == "" {
			return model, nil
		}
		model.feedback = errorTUIFeedback(err)
		return model, nil
	}

	if action.ID == tuiActionQuit {
		return model, tea.Quit
	}

	model.busy = true
	model.busyAction = action.Label
	model.spinnerTick = 0

	return model, tea.Batch(
		bubbleExecuteActionCmd(model.serviceName, model.status, choice, model.executeAction, model.collectStatus),
		bubbleSpinnerCmd(),
	)
}

func (model bubbleTUIModel) View() string {
	width := normalizeRenderWidth(model.width)
	if model.width <= 0 {
		width = currentTerminalSize().Cols
	}

	actions := tuiActionsForStatus(model.status)
	selected := selectedAction(actions, model.selected)
	footer := bubbleFooterLines(selected, actions, width, model.busy, model.busyAction)
	bodyHeight := model.height - len(footer) - 1
	if bodyHeight < 8 {
		bodyHeight = 8
	}

	feedback := model.feedback
	if model.busy {
		summary := fmt.Sprintf("%s %s", bubbleSpinnerFrames[model.spinnerTick], nonEmptyString(model.busyAction, "Working"))
		feedback = infoTUIFeedback(summary, "Waiting for the host to finish the requested action.")
	}

	body := renderPlainTUIViewForSize(model.status, feedback, width, bodyHeight)
	parts := []string{body}
	if len(footer) > 0 {
		parts = append(parts, "", strings.Join(footer, "\n"))
	}
	return strings.Join(parts, "\n")
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

func bubbleFooterLines(selected tuiAction, actions []tuiAction, width int, busy bool, busyAction string) []string {
	lines := make([]string, 0, 4)
	if selected.Key != "" {
		lines = appendWrappedFooter(lines, fmt.Sprintf("Selection : [%s] %s", selected.Key, selected.Label), width)
	}
	lines = appendWrappedFooter(lines, fmt.Sprintf("Controls  : up/down or j/k move, enter runs, direct keys: %s", bubbleShortcutSummary(actions)), width)
	if busy {
		lines = appendWrappedFooter(lines, fmt.Sprintf("Busy      : %s", nonEmptyString(busyAction, "Waiting for action result")), width)
	}
	return lines
}

func appendWrappedFooter(lines []string, text string, width int) []string {
	for _, line := range wrapPanelLine(text, maxInt(width, 20)) {
		lines = append(lines, line)
	}
	return lines
}

func bubbleShortcutSummary(actions []tuiAction) string {
	keys := make([]string, 0, len(actions))
	for _, action := range actions {
		keys = append(keys, action.Key)
	}
	return strings.Join(keys, " ")
}

func isDirectActionShortcut(status tuiStatus, key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	for _, action := range tuiActionsForStatus(status) {
		for _, alias := range action.Aliases {
			if normalized == alias {
				return true
			}
		}
	}
	return !status.Capabilities.Reload && (normalized == "4" || normalized == "reload")
}

func selectedAction(actions []tuiAction, selected int) tuiAction {
	if len(actions) == 0 {
		return tuiAction{}
	}
	selected = clampSelectedAction(selected, actions)
	return actions[selected]
}

func moveSelectedAction(selected, delta int, actions []tuiAction) int {
	if len(actions) == 0 {
		return 0
	}
	selected = clampSelectedAction(selected, actions)
	selected += delta
	if selected < 0 {
		selected = len(actions) - 1
	}
	if selected >= len(actions) {
		selected = 0
	}
	return selected
}

func clampSelectedAction(selected int, actions []tuiAction) int {
	if len(actions) == 0 {
		return 0
	}
	if selected < 0 {
		return 0
	}
	if selected >= len(actions) {
		return len(actions) - 1
	}
	return selected
}
