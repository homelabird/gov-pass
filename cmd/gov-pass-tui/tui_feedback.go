package main

import "strings"

type tuiFeedbackLevel string

const (
	tuiFeedbackOK    tuiFeedbackLevel = "OK"
	tuiFeedbackInfo  tuiFeedbackLevel = "INFO"
	tuiFeedbackError tuiFeedbackLevel = "ERROR"
)

type tuiFeedback struct {
	Level   tuiFeedbackLevel
	Summary string
	Detail  string
}

func readyTUIFeedback() tuiFeedback {
	return tuiFeedback{
		Level:   tuiFeedbackOK,
		Summary: "Ready.",
		Detail:  "Press Enter, Space, or click the ON/OFF button.",
	}
}

func infoTUIFeedback(summary, detail string) tuiFeedback {
	return tuiFeedback{
		Level:   tuiFeedbackInfo,
		Summary: strings.TrimSpace(summary),
		Detail:  strings.TrimSpace(detail),
	}
}

func successTUIFeedback(summary, detail string) tuiFeedback {
	return tuiFeedback{
		Level:   tuiFeedbackOK,
		Summary: strings.TrimSpace(summary),
		Detail:  strings.TrimSpace(detail),
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
	}
}

func (feedback tuiFeedback) isZero() bool {
	return strings.TrimSpace(feedback.Summary) == "" && strings.TrimSpace(feedback.Detail) == "" && strings.TrimSpace(string(feedback.Level)) == ""
}
