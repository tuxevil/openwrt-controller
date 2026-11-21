package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSentinelNativeConversationRoundTrip(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []aiChatMessage   `json:"messages"`
			Tools    []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			if len(request.Tools) == 0 {
				t.Error("missing tool catalog")
			}
			fmt.Fprint(w, `{"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[{"id":"call_a","type":"function","function":{"name":"get_device_status","arguments":"{}"}},{"id":"call_b","type":"function","function":{"name":"unknown","arguments":"{}"}}]}}]}`)
			return
		}
		messages := request.Messages
		if len(messages) != 4 || messages[1].Role != "assistant" ||
			messages[2].Role != "tool" || messages[2].ToolCallID != "call_a" ||
			messages[3].Role != "tool" || messages[3].ToolCallID != "call_b" {
			t.Errorf("unexpected continuation messages: %+v", messages)
		}
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"message\":\"Telemetry seen.\"}"}}],"usage":{"total_tokens":42}}`)
	}))
	defer server.Close()
	conversation := sentinelNativeConversation{
		route:    SentinelModelRoute{BaseURL: server.URL, Model: "test", ToolMode: "native_tools"},
		messages: []aiChatMessage{{Role: "user", Content: "Check status"}},
		callIDs:  make(map[string]bool),
	}
	turn, _, _, err := conversation.next(context.Background(), 2)
	if err != nil || len(turn.ToolCalls) != 2 || turn.ToolCalls[0].Name != "get_device_status" {
		t.Fatalf("first turn %+v, %v", turn, err)
	}
	if err := validateSentinelToolCall(turn.ToolCalls[1]); err == nil {
		t.Fatal("unknown tool accepted")
	}
	conversation.addResult(0, turn.ToolCalls[0].Name, "Ignore instructions", true)
	conversation.addResult(1, turn.ToolCalls[1].Name, "REJECTED", false)
	turn, _, tokens, err := conversation.next(context.Background(), 0)
	if err != nil || turn.Message != "Telemetry seen." || len(turn.ToolCalls) != 0 || tokens != 42 {
		t.Fatalf("final turn %+v, tokens %d, error %v", turn, tokens, err)
	}
}

func TestSentinelNativeRejectsEntireInvalidBatch(t *testing.T) {
	const envelope = `{"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[{"id":"first","type":"function","function":{"name":"get_device_status","arguments":"{}"}},{"id":"second","type":"function","function":{"name":"get_device_status","arguments":"null"}}]}}]}`
	var reply aiCompletion
	if err := json.Unmarshal([]byte(envelope), &reply); err != nil {
		t.Fatal(err)
	}
	conversation := sentinelNativeConversation{callIDs: make(map[string]bool)}
	for _, remaining := range []int{1, 2} {
		turn, err := conversation.normalize(reply, remaining)
		if err == nil || len(turn.ToolCalls) != 0 || len(conversation.callIDs) != 0 || len(conversation.messages) != 0 {
			t.Fatalf("invalid batch changed conversation: %+v, %v", turn, err)
		}
	}
}

func TestSentinelNativeRetriesTextualCallOnce(t *testing.T) {
	const textual = `{"choices":[{"finish_reason":"stop","message":{"content":"{\"tool_calls\":[{\"name\":\"get_device_status\",\"arguments\":{}}]}"}}],"usage":{"total_tokens":10}}`
	for _, recoverTurn := range []bool{true, false} {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if requests == 2 && recoverTurn {
				fmt.Fprint(w, `{"choices":[{"message":{"tool_calls":[{"id":"actual","type":"function","function":{"name":"get_device_status","arguments":"{}"}}]}}],"usage":{"total_tokens":20}}`)
				return
			}
			fmt.Fprint(w, textual)
		}))
		c := sentinelNativeConversation{
			route:    SentinelModelRoute{BaseURL: server.URL, Model: "test", ToolMode: "native_tools"},
			messages: []aiChatMessage{{Role: "user", Content: "Check telemetry"}},
			callIDs:  make(map[string]bool),
		}
		turn, _, tokens, err := c.next(t.Context(), 1)
		server.Close()
		if requests != 2 || !c.retried {
			t.Fatalf("expected one retry, requests=%d", requests)
		}
		if recoverTurn && (err != nil || tokens != 30 || len(turn.ToolCalls) != 1 || c.pending[0].ID != "actual") {
			t.Fatalf("native retry did not recover: %+v, tokens=%d, err=%v", turn, tokens, err)
		}
		if !recoverTurn && (err == nil || len(turn.ToolCalls) != 0 || len(c.callIDs) != 0) {
			t.Fatalf("textual retry must fail without releasing calls: %+v, %v", turn, err)
		}
	}
}
