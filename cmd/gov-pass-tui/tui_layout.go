package main

import (
	"fmt"
	"strings"
	"unicode"
)

type panel struct {
	Title      string
	Lines      []string
	BorderChar rune
}

type plainLayoutProfile struct {
	TwoColumns         bool
	CompactActions     bool
	ShowRawState       bool
	ShowHints          bool
	WarningDetailLimit int
	FeedbackDetail     bool
}

func renderPlainTUIView(status tuiStatus, feedback tuiFeedback) string {
	size := currentTerminalSize()
	return renderPlainTUIViewForSize(status, feedback, size.Cols, size.Rows)
}

func renderPlainTUIViewForWidth(status tuiStatus, feedback tuiFeedback, width int) string {
	return renderPlainTUIViewForSize(status, feedback, width, currentTerminalSize().Rows)
}

func renderPlainTUIViewForSize(status tuiStatus, feedback tuiFeedback, width, height int) string {
	width = normalizeRenderWidth(width)
	height = normalizeRenderHeight(height)

	profile := newPlainLayoutProfile(width, height)
	overviewPanel := panel{
		Title:      fmt.Sprintf("GOV-PASS OPERATOR PANEL [%s]", healthLabel(status.Health)),
		Lines:      overviewPanelLines(status, profile),
		BorderChar: '=',
	}
	actionsPanel := panel{
		Title: "ACTIONS",
		Lines: actionPanelLines(status, profile),
	}
	resultPanel := panel{
		Title: "LAST RESULT",
		Lines: feedbackPanelLines(feedback, profile),
	}
	hintsPanel := panel{
		Title: "HINTS",
		Lines: hintPanelLines(status),
	}

	var warningPanel *panel
	if strings.TrimSpace(status.WarningSummary) != "" {
		warningPanel = &panel{
			Title: "WARNINGS",
			Lines: warningPanelLines(status, profile),
		}
	}

	var lines []string
	if profile.TwoColumns {
		gap := 2
		leftWidth := (width - gap) / 2
		rightWidth := width - gap - leftWidth

		leftPanels := []panel{overviewPanel, actionsPanel}
		rightPanels := []panel{resultPanel}
		if warningPanel != nil {
			rightPanels = append(rightPanels, *warningPanel)
		}
		if profile.ShowHints {
			rightPanels = append(rightPanels, hintsPanel)
		}

		lines = joinColumns(
			renderPanelStack(leftPanels, leftWidth),
			renderPanelStack(rightPanels, rightWidth),
			leftWidth,
			rightWidth,
			gap,
		)
	} else {
		panels := []panel{overviewPanel, actionsPanel, resultPanel}
		if warningPanel != nil {
			panels = append(panels, *warningPanel)
		}
		if profile.ShowHints {
			panels = append(panels, hintsPanel)
		}
		lines = renderPanelStack(panels, width)
	}

	lines = trimRenderedLines(lines, height)
	return strings.Join(lines, "\n")
}

func buildCompactStatusSummary(status tuiStatus, feedback tuiFeedback) string {
	lines := []string{
		fmt.Sprintf("Health: %s", healthLabel(status.Health)),
		fmt.Sprintf("Service: %s", nonEmptyString(status.ServiceName, "-")),
		fmt.Sprintf("State: %s", status.StateLabel),
		fmt.Sprintf("Boot: %s", status.BootLabel),
		fmt.Sprintf("Reload: %s", capabilityLabel(status.Capabilities.Reload)),
	}
	if strings.TrimSpace(status.WarningSummary) != "" {
		lines = append(lines, fmt.Sprintf("Warning: %s", status.WarningSummary))
	}
	if summary := strings.TrimSpace(feedback.Summary); summary != "" {
		lines = append(lines, fmt.Sprintf("Last: %s", summary))
	}
	lines = append(lines, "", "Choose an action:")
	return strings.Join(lines, "\n")
}

func newPlainLayoutProfile(width, height int) plainLayoutProfile {
	profile := plainLayoutProfile{
		TwoColumns:         width >= 96 && height >= 18,
		CompactActions:     width < 72 || height < 24,
		ShowRawState:       width >= 56 && height >= 16,
		ShowHints:          height >= 18,
		WarningDetailLimit: 0,
		FeedbackDetail:     height >= 14,
	}

	switch {
	case height >= 30:
		profile.WarningDetailLimit = 4
	case height >= 24:
		profile.WarningDetailLimit = 2
	case height >= 18:
		profile.WarningDetailLimit = 1
	}

	return profile
}

func overviewPanelLines(status tuiStatus, profile plainLayoutProfile) []string {
	lines := []string{
		fmt.Sprintf("Platform : %s", nonEmptyString(status.Platform, "Unknown")),
		fmt.Sprintf("Service  : %s", nonEmptyString(status.ServiceName, "-")),
		fmt.Sprintf("Health   : %s", healthLabel(status.Health)),
		fmt.Sprintf("Summary  : %s", nonEmptyString(status.Summary, "Service state is unknown.")),
		fmt.Sprintf("State    : %s", status.StateLabel),
		fmt.Sprintf("Boot     : %s", status.BootLabel),
		fmt.Sprintf("Reload   : %s", capabilityLabel(status.Capabilities.Reload)),
		fmt.Sprintf("Updated  : %s", status.UpdatedAt.Format("2006-01-02 15:04:05")),
	}
	if profile.ShowRawState {
		lines = append(lines[0:5], append([]string{fmt.Sprintf("Raw      : %s", nonEmptyString(status.RawState, "unknown"))}, lines[5:]...)...)
	}
	return lines
}

func feedbackPanelLines(feedback tuiFeedback, profile plainLayoutProfile) []string {
	summary := nonEmptyString(feedback.Summary, "Ready.")
	lines := []string{
		fmt.Sprintf("Level    : %s", nonEmptyString(string(feedback.Level), string(tuiFeedbackOK))),
		fmt.Sprintf("Summary  : %s", summary),
	}
	if profile.FeedbackDetail && strings.TrimSpace(feedback.Detail) != "" {
		lines = append(lines, prefixedLines(feedback.Detail, "Detail   : ", "           ")...)
	}
	return lines
}

func warningPanelLines(status tuiStatus, profile plainLayoutProfile) []string {
	lines := []string{
		fmt.Sprintf("Summary  : %s", status.WarningSummary),
	}

	if profile.WarningDetailLimit <= 0 {
		return lines
	}

	limit := minInt(profile.WarningDetailLimit, len(status.WarningDetails))
	for i := 0; i < limit; i++ {
		lines = append(lines, prefixedLines(status.WarningDetails[i], "Detail   : ", "           ")...)
	}

	if len(status.WarningDetails) > limit {
		lines = append(lines, fmt.Sprintf("Detail   : +%d more issue(s)", len(status.WarningDetails)-limit))
	}

	return lines
}

func actionPanelLines(status tuiStatus, profile plainLayoutProfile) []string {
	actions := tuiActionsForStatus(status)
	lines := make([]string, 0, len(actions)*2)
	for _, action := range actions {
		if profile.CompactActions {
			lines = append(lines, fmt.Sprintf("[%s] %s", action.Key, action.Label))
			continue
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", action.Key, action.Label))
		lines = append(lines, "    "+action.Detail)
	}
	return lines
}

func hintPanelLines(status tuiStatus) []string {
	lines := []string{
		"Input    : Type the key shown in brackets and press Enter.",
		"Refresh  : Use refresh after external service changes.",
	}
	if !status.Capabilities.Reload {
		lines = append(lines, "Reload   : Use restart on this platform.")
	}
	return lines
}

func capabilityLabel(supported bool) string {
	if supported {
		return "Supported"
	}
	return "Restart required"
}

func renderPanelStack(panels []panel, width int) []string {
	lines := make([]string, 0, len(panels)*8)
	for _, section := range panels {
		lines = append(lines, renderPanelLines(section, width)...)
	}
	return lines
}

func renderPanelLines(section panel, width int) []string {
	borderChar := section.BorderChar
	if borderChar == 0 {
		borderChar = '-'
	}
	horizontal := strings.Repeat(string(borderChar), width-2)

	lines := []string{
		"+" + horizontal + "+",
	}
	for _, line := range wrapPanelLine(strings.TrimSpace(section.Title), width-3) {
		lines = append(lines, formatPaddedLine(width, " "+line))
	}
	lines = append(lines, "+"+horizontal+"+")

	for _, line := range section.Lines {
		for _, wrapped := range wrapPanelLine(line, width-3) {
			lines = append(lines, formatPaddedLine(width, " "+wrapped))
		}
	}

	return lines
}

func joinColumns(left, right []string, leftWidth, rightWidth, gap int) []string {
	maxLines := maxInt(len(left), len(right))
	out := make([]string, 0, maxLines)
	gutter := strings.Repeat(" ", gap)
	leftBlank := strings.Repeat(" ", leftWidth)
	rightBlank := strings.Repeat(" ", rightWidth)

	for i := 0; i < maxLines; i++ {
		leftLine := leftBlank
		rightLine := rightBlank
		if i < len(left) {
			leftLine = left[i]
		}
		if i < len(right) {
			rightLine = right[i]
		}
		out = append(out, leftLine+gutter+rightLine)
	}

	return out
}

func trimRenderedLines(lines []string, height int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	if height < 2 {
		return lines[:height]
	}

	trimmed := append([]string(nil), lines[:height-1]...)
	last := lines[height-2]
	width := runeLen(last)
	trimmed = append(trimmed, truncateLineToWidth("... output truncated for terminal height ...", width))
	return trimmed
}

func formatPaddedLine(width int, content string) string {
	if runeLen(content) > width-2 {
		content = string([]rune(content)[:width-2])
	}
	padding := width - 2 - runeLen(content)
	return "|" + content + strings.Repeat(" ", maxInt(padding, 0)) + "|"
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
	switch {
	case height <= 0:
		return 28
	case height < 10:
		return 10
	default:
		return height
	}
}

func countWrappedTextLines(text string, width int) int {
	width = maxInt(width, 1)
	total := 0
	for _, paragraph := range strings.Split(strings.TrimSpace(text), "\n") {
		total += len(wrapPanelLine(paragraph, width))
	}
	if total == 0 {
		return 1
	}
	return total
}

func wrapPanelLine(text string, width int) []string {
	width = maxInt(width, 1)
	text = strings.TrimRightFunc(text, unicode.IsSpace)
	if text == "" {
		return []string{""}
	}

	continuation := continuationPrefix(text)
	remaining := []rune(text)
	lines := make([]string, 0, 4)
	currentPrefix := ""

	for len(remaining) > 0 {
		limit := width - runeLen(currentPrefix)
		if limit < 1 {
			limit = width
		}
		if len(remaining) <= limit {
			lines = append(lines, currentPrefix+string(remaining))
			break
		}

		split := lastWhitespaceWithin(remaining, limit)
		if split == 0 {
			split = limit
		}

		line := strings.TrimRightFunc(string(remaining[:split]), unicode.IsSpace)
		if line == "" {
			line = string(remaining[:limit])
			split = limit
		}
		lines = append(lines, currentPrefix+line)
		remaining = trimLeadingSpaceRunes(remaining[split:])
		currentPrefix = continuation
	}

	return lines
}

func continuationPrefix(text string) string {
	if idx := strings.Index(text, ": "); idx > 0 && idx <= 16 {
		return strings.Repeat(" ", idx+2)
	}
	if strings.HasPrefix(text, "[") {
		if idx := strings.Index(text, "] "); idx > 0 && idx <= 6 {
			return strings.Repeat(" ", idx+2)
		}
	}
	return ""
}

func lastWhitespaceWithin(text []rune, limit int) int {
	for i := limit; i > 0; i-- {
		if unicode.IsSpace(text[i-1]) {
			return i
		}
	}
	return 0
}

func trimLeadingSpaceRunes(text []rune) []rune {
	for len(text) > 0 && unicode.IsSpace(text[0]) {
		text = text[1:]
	}
	return text
}

func runeLen(text string) int {
	return len([]rune(text))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func prefixedLines(text, firstPrefix, nextPrefix string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return []string{firstPrefix + "-"}
	}
	rawLines := strings.Split(trimmed, "\n")
	lines := make([]string, 0, len(rawLines))
	for i, raw := range rawLines {
		prefix := nextPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		lines = append(lines, prefix+strings.TrimSpace(raw))
	}
	return lines
}

func nonEmptyString(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}
