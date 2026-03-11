package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

func logInfo(eventID, msg string, kv ...any) {
	log.Print(formatLogEvent("info", eventID, msg, kv...))
}

func logWarn(eventID, msg string, kv ...any) {
	log.Print(formatLogEvent("warn", eventID, msg, kv...))
}

func logError(eventID, msg string, err error, kv ...any) {
	if err != nil {
		kv = append(kv, "err", err)
	}
	log.Print(formatLogEvent("error", eventID, msg, kv...))
}

func formatLogEvent(level, eventID, msg string, kv ...any) string {
	var b strings.Builder
	appendLogfmtField(&b, "level", level)
	appendLogfmtField(&b, "event", eventID)
	appendLogfmtField(&b, "msg", msg)
	for i := 0; i < len(kv); i += 2 {
		if i+1 >= len(kv) {
			appendLogfmtField(&b, fmt.Sprintf("arg%d", i/2+1), kv[i])
			break
		}
		appendLogfmtField(&b, sanitizeLogKey(fmt.Sprint(kv[i])), kv[i+1])
	}
	return b.String()
}

func appendLogfmtField(b *strings.Builder, key string, value any) {
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(sanitizeLogKey(key))
	b.WriteByte('=')
	b.WriteString(formatLogValue(value))
}

func sanitizeLogKey(key string) string {
	if key == "" {
		return "field"
	}
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "field"
	}
	return b.String()
}

func formatLogValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return quoteLogValue(v)
	case error:
		return quoteLogValue(v.Error())
	case fmt.Stringer:
		return quoteLogValue(v.String())
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64:
		return fmt.Sprint(v)
	default:
		return quoteLogValue(fmt.Sprint(v))
	}
}

func quoteLogValue(value string) string {
	if value == "" {
		return `""`
	}
	for _, r := range value {
		if r <= ' ' || r == '=' || r == '"' {
			return strconv.Quote(value)
		}
	}
	return value
}
