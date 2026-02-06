package llm

import (
	"bufio"
	"io"
	"strings"
)

// SSEEvent represents a single Server-Sent Event.
type SSEEvent struct {
	Event string
	Data  string
}

// ParseSSE reads an SSE stream and yields events.
// It handles the standard SSE format: lines prefixed with "data: ".
// Closes the returned channel when the stream ends or [DONE] is received.
func ParseSSE(reader io.Reader) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(reader)
		// Increase buffer size for large streaming chunks
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var currentEvent string
		var dataLines []string

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				// Empty line = end of event
				if len(dataLines) > 0 {
					data := strings.Join(dataLines, "\n")
					if data == "[DONE]" {
						return
					}
					ch <- SSEEvent{
						Event: currentEvent,
						Data:  data,
					}
				}
				currentEvent = ""
				dataLines = nil
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				dataLines = append(dataLines, line[6:])
			} else if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, line[5:])
			} else if strings.HasPrefix(line, "event: ") {
				currentEvent = line[7:]
			} else if strings.HasPrefix(line, "event:") {
				currentEvent = line[6:]
			}
			// Ignore other lines (comments starting with :, etc.)
		}

		// Flush any remaining data
		if len(dataLines) > 0 {
			data := strings.Join(dataLines, "\n")
			if data != "[DONE]" {
				ch <- SSEEvent{
					Event: currentEvent,
					Data:  data,
				}
			}
		}
	}()
	return ch
}
