package main

import (
	"fmt"
	"strings"
	"unicode"
)

type tuiStatus struct {
	Platform    string
	ServiceName string
	State       string
	Active      bool
	Enabled     bool
	Warning     string
}

type tuiAction struct {
	Key    string
	Label  string
	Detail string
}

var defaultTUIActions = []tuiAction{
	{Key: "1", Label: "Toggle service", Detail: "Start when stopped, stop when running"},
	{Key: "2", Label: "Restart service", Detail: "Restart the runtime without changing boot mode"},
	{Key: "3", Label: "Toggle boot", Detail: "Enable or disable startup at boot"},
	{Key: "r", Label: "Refresh", Detail: "Re-read service state from the host"},
	{Key: "q", Label: "Quit", Detail: "Leave the operator panel"},
}

func renderPlainTUIView(status tuiStatus, note string) string {
	return renderPlainTUIViewForWidth(status, note, currentTerminalSize().Cols)
}

func renderPlainTUIViewForWidth(status tuiStatus, note string, width int) string {
	sections := []panel{
		{
			Title: "GOV-PASS CONTROL",
			Lines: []string{
				fmt.Sprintf("Platform : %s", strings.ToUpper(nonEmptyString(status.Platform, "unknown"))),
				fmt.Sprintf("Service  : %s", nonEmptyString(status.ServiceName, "-")),
				fmt.Sprintf("State    : %s", strings.ToUpper(nonEmptyString(status.State, "unknown"))),
				fmt.Sprintf("Signals  : %s %s", boolBadge(status.Active, "ACTIVE", "IDLE"), boolBadge(status.Enabled, "BOOT ON", "BOOT OFF")),
			},
		},
		{
			Title: "ACTIONS",
			Lines: formatActionLines(defaultTUIActions),
		},
		{
			Title: "MESSAGE",
			Lines: append([]string{fmt.Sprintf("Tone    : %s", noteTone(note))}, prefixedLines(note, "Text    : ", "          ")...),
		},
	}

	if strings.TrimSpace(status.Warning) != "" {
		sections = append(sections[:1], append([]panel{{
			Title: "STATUS WARNING",
			Lines: prefixedLines(status.Warning, "Notice  : ", "          "),
		}}, sections[1:]...)...)
	}

	return renderPanels(sections, normalizeRenderWidth(width))
}

func buildCompactStatusSummary(status tuiStatus) string {
	lines := []string{
		fmt.Sprintf("Platform: %s", nonEmptyString(status.Platform, "unknown")),
		fmt.Sprintf("Service: %s", nonEmptyString(status.ServiceName, "-")),
		fmt.Sprintf("State: %s", nonEmptyString(status.State, "unknown")),
		fmt.Sprintf("Signals: %s %s", boolBadge(status.Active, "ACTIVE", "IDLE"), boolBadge(status.Enabled, "BOOT ON", "BOOT OFF")),
		"",
		"Choose an action:",
	}
	if strings.TrimSpace(status.Warning) != "" {
		warningLines := append(prefixedLines(status.Warning, "Warning: ", "         "), "")
		lines = append(lines[:4], append(warningLines, lines[4:]...)...)
	}
	return strings.Join(lines, "\n")
}

type panel struct {
	Title string
	Lines []string
}

func renderPanels(panels []panel, width int) string {
	var b strings.Builder
	for i, section := range panels {
		borderChar := '-'
		if i == 0 {
			borderChar = '='
		}
		writePanel(&b, width, borderChar, section.Title, section.Lines)
	}
	return b.String()
}

func writePanel(b *strings.Builder, width int, borderChar rune, title string, lines []string) {
	horizontal := strings.Repeat(string(borderChar), width-2)
	b.WriteString("+")
	b.WriteString(horizontal)
	b.WriteString("+\n")
	for _, line := range wrapPanelLine(strings.TrimSpace(title), width-3) {
		writePaddedLine(b, width, " "+line)
	}
	b.WriteString("+")
	b.WriteString(horizontal)
	b.WriteString("+\n")
	for _, line := range lines {
		for _, wrapped := range wrapPanelLine(line, width-3) {
			writePaddedLine(b, width, " "+wrapped)
		}
	}
}

func writePaddedLine(b *strings.Builder, width int, content string) {
	if runeLen(content) > width-2 {
		content = string([]rune(content)[:width-2])
	}
	padding := width - 2 - runeLen(content)
	b.WriteString("|")
	b.WriteString(content)
	if padding > 0 {
		b.WriteString(strings.Repeat(" ", padding))
	}
	b.WriteString("|\n")
}

func normalizeRenderWidth(width int) int {
	switch {
	case width <= 0:
		return 96
	case width < 16:
		return 16
	case width > 120:
		return 120
	default:
		return width
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

func formatActionLines(actions []tuiAction) []string {
	lines := make([]string, 0, len(actions))
	for _, action := range actions {
		lines = append(lines, fmt.Sprintf("[%s] %-14s %s", action.Key, action.Label, action.Detail))
	}
	return lines
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

func boolBadge(ok bool, onText, offText string) string {
	if ok {
		return "[" + onText + "]"
	}
	return "[" + offText + "]"
}

func noteTone(note string) string {
	trimmed := strings.TrimSpace(strings.ToLower(note))
	switch {
	case trimmed == "", trimmed == "ready.", trimmed == "done.":
		return "OK"
	case strings.HasPrefix(trimmed, "error:"):
		return "ERROR"
	case strings.Contains(trimmed, "warning"):
		return "WARN"
	default:
		return "INFO"
	}
}

func nonEmptyString(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}
