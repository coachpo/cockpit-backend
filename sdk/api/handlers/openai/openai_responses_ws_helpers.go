package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coachpo/cockpit-backend/internal/interfaces"
	"github.com/coachpo/cockpit-backend/sdk/api/handlers"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func responseCompletedOutputFromPayload(payload []byte) []byte {
	output := gjson.GetBytes(payload, "response.output")
	if output.Exists() && output.IsArray() {
		return bytes.Clone([]byte(output.Raw))
	}
	return []byte("[]")
}

func websocketJSONPayloadsFromChunk(chunk []byte) [][]byte {
	payloads := make([][]byte, 0, 2)
	lines := bytes.Split(chunk, []byte("\n"))
	for i := range lines {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 || bytes.HasPrefix(line, []byte("event:")) {
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimSpace(line[len("data:"):])
		}
		if len(line) == 0 || bytes.Equal(line, []byte(wsDoneMarker)) {
			continue
		}
		if json.Valid(line) {
			payloads = append(payloads, bytes.Clone(line))
		}
	}

	if len(payloads) > 0 {
		return payloads
	}

	trimmed := bytes.TrimSpace(chunk)
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		trimmed = bytes.TrimSpace(trimmed[len("data:"):])
	}
	if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte(wsDoneMarker)) && json.Valid(trimmed) {
		payloads = append(payloads, bytes.Clone(trimmed))
	}
	return payloads
}

func writeResponsesWebsocketError(conn *websocket.Conn, errMsg *interfaces.ErrorMessage) ([]byte, error) {
	status := http.StatusInternalServerError
	errText := http.StatusText(status)
	if errMsg != nil {
		if errMsg.StatusCode > 0 {
			status = errMsg.StatusCode
			errText = http.StatusText(status)
		}
		if errMsg.Error != nil && strings.TrimSpace(errMsg.Error.Error()) != "" {
			errText = errMsg.Error.Error()
		}
	}

	body := handlers.BuildErrorResponseBody(status, errText)
	payload := []byte(`{}`)
	var errSet error
	payload, errSet = sjson.SetBytes(payload, "type", wsEventTypeError)
	if errSet != nil {
		return nil, errSet
	}
	payload, errSet = sjson.SetBytes(payload, "status", status)
	if errSet != nil {
		return nil, errSet
	}

	if errMsg != nil && errMsg.Addon != nil {
		headers := []byte(`{}`)
		hasHeaders := false
		for key, values := range errMsg.Addon {
			if len(values) == 0 {
				continue
			}
			headerPath := strings.ReplaceAll(strings.ReplaceAll(key, `\`, `\\`), ".", `\.`)
			headers, errSet = sjson.SetBytes(headers, headerPath, values[0])
			if errSet != nil {
				return nil, errSet
			}
			hasHeaders = true
		}
		if hasHeaders {
			payload, errSet = sjson.SetRawBytes(payload, "headers", headers)
			if errSet != nil {
				return nil, errSet
			}
		}
	}

	if len(body) > 0 && json.Valid(body) {
		errorNode := gjson.GetBytes(body, "error")
		if errorNode.Exists() {
			payload, errSet = sjson.SetRawBytes(payload, "error", []byte(errorNode.Raw))
		} else {
			payload, errSet = sjson.SetRawBytes(payload, "error", body)
		}
		if errSet != nil {
			return nil, errSet
		}
	}

	if !gjson.GetBytes(payload, "error").Exists() {
		payload, errSet = sjson.SetBytes(payload, "error.type", "server_error")
		if errSet != nil {
			return nil, errSet
		}
		payload, errSet = sjson.SetBytes(payload, "error.message", errText)
		if errSet != nil {
			return nil, errSet
		}
	}

	return payload, conn.WriteMessage(websocket.TextMessage, payload)
}

func appendWebsocketEvent(builder *strings.Builder, eventType string, payload []byte) {
	if builder == nil {
		return
	}
	if builder.Len() >= wsBodyLogMaxSize {
		return
	}
	trimmedPayload := bytes.TrimSpace(payload)
	if len(trimmedPayload) == 0 {
		return
	}
	if builder.Len() > 0 {
		if !appendWebsocketLogString(builder, "\n") {
			return
		}
	}
	if !appendWebsocketLogString(builder, "websocket.") {
		return
	}
	if !appendWebsocketLogString(builder, eventType) {
		return
	}
	if !appendWebsocketLogString(builder, "\n") {
		return
	}
	if !appendWebsocketLogBytes(builder, trimmedPayload, len(wsBodyLogTruncated)) {
		appendWebsocketLogString(builder, wsBodyLogTruncated)
		return
	}
	appendWebsocketLogString(builder, "\n")
}

func appendWebsocketLogString(builder *strings.Builder, value string) bool {
	if builder == nil {
		return false
	}
	remaining := wsBodyLogMaxSize - builder.Len()
	if remaining <= 0 {
		return false
	}
	if len(value) <= remaining {
		builder.WriteString(value)
		return true
	}
	builder.WriteString(value[:remaining])
	return false
}

func appendWebsocketLogBytes(builder *strings.Builder, value []byte, reserveForSuffix int) bool {
	if builder == nil {
		return false
	}
	remaining := wsBodyLogMaxSize - builder.Len()
	if remaining <= 0 {
		return false
	}
	if len(value) <= remaining {
		builder.Write(value)
		return true
	}
	limit := remaining - reserveForSuffix
	if limit < 0 {
		limit = 0
	}
	if limit > len(value) {
		limit = len(value)
	}
	builder.Write(value[:limit])
	return false
}

func websocketPayloadEventType(payload []byte) string {
	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if eventType == "" {
		return "-"
	}
	return eventType
}

func websocketPayloadPreview(payload []byte) string {
	trimmedPayload := bytes.TrimSpace(payload)
	if len(trimmedPayload) == 0 {
		return "<empty>"
	}
	preview := trimmedPayload
	if len(preview) > wsPayloadLogMaxSize {
		preview = preview[:wsPayloadLogMaxSize]
	}
	previewText := strings.ReplaceAll(string(preview), "\n", "\\n")
	previewText = strings.ReplaceAll(previewText, "\r", "\\r")
	if len(trimmedPayload) > wsPayloadLogMaxSize {
		return fmt.Sprintf("%s...(truncated,total=%d)", previewText, len(trimmedPayload))
	}
	return previewText
}

func setWebsocketRequestBody(c *gin.Context, body string) {
	if c == nil {
		return
	}
	trimmedBody := strings.TrimSpace(body)
	if trimmedBody == "" {
		return
	}
	c.Set(wsRequestBodyKey, []byte(trimmedBody))
}

func markAPIResponseTimestamp(c *gin.Context) {
	if c == nil {
		return
	}
	if _, exists := c.Get("API_RESPONSE_TIMESTAMP"); exists {
		return
	}
	c.Set("API_RESPONSE_TIMESTAMP", time.Now())
}
