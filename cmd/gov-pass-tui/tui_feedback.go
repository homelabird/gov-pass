package main

import "strings"

type tuiFeedbackLevel string

const (
	tuiFeedbackOK    tuiFeedbackLevel = "OK"
	tuiFeedbackInfo  tuiFeedbackLevel = "INFO"
	tuiFeedbackWarn  tuiFeedbackLevel = "WARN"
	tuiFeedbackError tuiFeedbackLevel = "ERROR"
)

type tuiFeedback struct {
	Level   tuiFeedbackLevel
	Summary string
	Detail  string
	Popup   bool
}

func readyTUIFeedback() tuiFeedback {
	return tuiFeedback{
		Level:   tuiFeedbackOK,
		Summary: "Ready.",
		Detail:  "Select an action to manage the service.",
	}
}

func infoTUIFeedback(summary, detail string) tuiFeedback {
	return tuiFeedback{
		Level:   tuiFeedbackInfo,
		Summary: strings.TrimSpace(summary),
		Detail:  strings.TrimSpace(detail),
	}
}

func successTUIFeedback(summary, detail string, popup bool) tuiFeedback {
	return tuiFeedback{
		Level:   tuiFeedbackOK,
		Summary: strings.TrimSpace(summary),
		Detail:  strings.TrimSpace(detail),
		Popup:   popup,
	}
}

func errorTUIFeedback(err error) tuiFeedback {
	detail := ""
	if err != nil {
		detail = strings.TrimSpace(err.Error())
	}
	return tuiFeedback{
		Level:   tuiFeedbackError,
		Summary: "Action failed.",
		Detail:  detail,
		Popup:   true,
	}
}

func (feedback tuiFeedback) dialogTitle() string {
	switch feedback.Level {
	case tuiFeedbackError:
		return "gov-pass error"
	case tuiFeedbackWarn:
		return "gov-pass warning"
	default:
		return "gov-pass"
	}
}

func (feedback tuiFeedback) text() string {
	lines := make([]string, 0, 2)
	if summary := strings.TrimSpace(feedback.Summary); summary != "" {
		lines = append(lines, summary)
	}
	if detail := strings.TrimSpace(feedback.Detail); detail != "" {
		lines = append(lines, detail)
	}
	return strings.Join(lines, "\n")
}

func (feedback tuiFeedback) isZero() bool {
	return strings.TrimSpace(feedback.Summary) == "" && strings.TrimSpace(feedback.Detail) == "" && strings.TrimSpace(string(feedback.Level)) == ""
}
