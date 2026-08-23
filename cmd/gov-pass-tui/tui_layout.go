package main

import "strings"

const toggleButtonRow = 8

type toggleButtonBounds struct {
	left  int
	right int
	row   int
}

func (bounds toggleButtonBounds) contains(x, y int) bool {
	return y == bounds.row && x >= bounds.left && x < bounds.right
}

func renderToggleTUIViewForSize(status tuiStatus, feedback tuiFeedback, width, height int, busy bool) string {
	width = normalizeRenderWidth(width)
	height = normalizeRenderHeight(height)
	panelWidth := minInt(width, 64)
	contentWidth := panelWidth - 2
	stateLabel, buttonLabel := toggleButtonPresentation(status, busy)

	lines := []string{
		"+" + strings.Repeat("=", contentWidth) + "+",
		"|" + centeredText("GOV-PASS", contentWidth) + "|",
		"+" + strings.Repeat("=", contentWidth) + "+",
		"|" + centeredText(nonEmptyString(status.Platform, "System")+" · "+nonEmptyString(status.ServiceName, "gov-pass"), contentWidth) + "|",
		"|" + strings.Repeat(" ", contentWidth) + "|",
		"|" + centeredText("STATUS: "+stateLabel, contentWidth) + "|",
		"|" + strings.Repeat(" ", contentWidth) + "|",
		"|" + strings.Repeat(" ", contentWidth) + "|",
		"|" + centeredText(buttonLabel, contentWidth) + "|",
		"|" + strings.Repeat(" ", contentWidth) + "|",
		"|" + centeredText("Enter / Space / click", contentWidth) + "|",
		"|" + centeredText("q: quit", contentWidth) + "|",
	}

	if summary := strings.TrimSpace(feedback.Summary); summary != "" && summary != "Ready." {
		lines = append(lines, "|"+centeredText(summary, contentWidth)+"|")
	}
	if feedback.Level == tuiFeedbackError && strings.TrimSpace(feedback.Detail) != "" {
		lines = append(lines, "|"+centeredText(feedback.Detail, contentWidth)+"|")
	}
	if strings.TrimSpace(status.WarningSummary) != "" {
		lines = append(lines, "|"+centeredText("Warning: "+status.WarningSummary, contentWidth)+"|")
	}
	lines = append(lines, "+"+strings.Repeat("=", contentWidth)+"+")
	lines = trimRenderedLines(lines, height)

	leftPad := strings.Repeat(" ", (width-panelWidth)/2)
	for i := range lines {
		lines[i] = leftPad + lines[i]
	}
	return strings.Join(lines, "\n")
}

func toggleButtonPresentation(status tuiStatus, busy bool) (state, button string) {
	if busy || serviceStateBusy(status.RawState) {
		return "WORKING", "[ WORKING... ]"
	}
	if action, ok := knownServiceToggleAction(status.RawState); ok {
		if action == "stop" {
			return "ON", "[ TURN OFF ]"
		}
		return "OFF", "[ TURN ON ]"
	}
	if status.ActiveKnown {
		if status.Active {
			return "ON", "[ TURN OFF ]"
		}
		return "OFF", "[ TURN ON ]"
	}
	return "UNKNOWN", "[ TOGGLE ]"
}

func toggleButtonBoundsForSize(status tuiStatus, width int, busy bool) toggleButtonBounds {
	width = normalizeRenderWidth(width)
	panelWidth := minInt(width, 64)
	contentWidth := panelWidth - 2
	_, label := toggleButtonPresentation(status, busy)
	left := (width-panelWidth)/2 + 1 + (contentWidth-runeLen(label))/2
	return toggleButtonBounds{left: left, right: left + runeLen(label), row: toggleButtonRow}
}

func centeredText(text string, width int) string {
	text = strings.TrimSpace(text)
	if runeLen(text) > width {
		text = string([]rune(text)[:width])
	}
	left := (width - runeLen(text)) / 2
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", width-left-runeLen(text))
}

func trimRenderedLines(lines []string, height int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	if height < 2 {
		return lines[:height]
	}

	trimmed := append([]string(nil), lines[:height-1]...)
	width := runeLen(lines[height-2])
	return append(trimmed, truncateLineToWidth("... output truncated for terminal height ...", width))
}

func truncateLineToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if runeLen(text) > width {
		return string([]rune(text)[:width])
	}
	return text + strings.Repeat(" ", width-runeLen(text))
}

func normalizeRenderWidth(width int) int {
	switch {
	case width <= 0:
		return 96
	case width < 20:
		return 20
	case width > 140:
		return 140
	default:
		return width
	}
}

func normalizeRenderHeight(height int) int {
	if height <= 0 {
		return 28
	}
	if height < 10 {
		return 10
	}
	return height
}

func runeLen(text string) int {
	return len([]rune(text))
}

func nonEmptyString(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}
