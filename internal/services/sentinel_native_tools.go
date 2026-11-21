package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

type aiChatMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content"`
	ToolCalls  []aiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type aiToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function aiToolFunction `json:"function"`
}

type aiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type sentinelNativeConversation struct {
	route    SentinelModelRoute
	messages []aiChatMessage
	callIDs  map[string]bool
	pending  []aiToolCall
}

func newSentinelNativeConversation(class SentinelReasoningClass, prompt string) *sentinelNativeConversation {
	settings := database.GetPlatformSettings()
	for _, candidate := range sentinelReasoningRouteChain(class) {
		route, err := resolveSentinelModelRouteWithEnv(candidate, settings, os.Getenv)
		if err != nil {
			continue
		}
		if route.ToolMode != "native_tools" {
			return nil
		}
		return &sentinelNativeConversation{
			route: route, callIDs: make(map[string]bool),
			messages: []aiChatMessage{
				{Role: "system", Content: sentinelNativeSystemPrompt()},
				{Role: "user", Content: prompt},
			},
		}
	}
	return nil
}

func sentinelNativeSystemPrompt() string {
	return sentinelAgentInstructions + "\nUse native function calls to request evidence. Return the final answer as a JSON object with message and an optional proposal (summary, device_id, config, commands, health_checks). Do not put tool_calls in JSON content.\n" + sentinelAgentEvidenceGuidance
}

func sentinelNativeCatalog() []map[string]any {
	var tools []map[string]any
	for _, descriptor := range SentinelToolDescriptors() {
		if descriptor.SideEffectClass != SentinelToolSideEffectNone {
			continue
		}
		properties := make(map[string]any, len(descriptor.InputSchema.Properties))
		for name, property := range descriptor.InputSchema.Properties {
			schema := map[string]any{"type": property.Type, "description": property.Description}
			if property.MaxLength > 0 {
				schema["maxLength"] = property.MaxLength
			}
			if property.Minimum != nil {
				schema["minimum"] = *property.Minimum
			}
			if property.Maximum != nil {
				schema["maximum"] = *property.Maximum
			}
			properties[name] = schema
		}
		parameters := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(descriptor.InputSchema.Required) > 0 {
			parameters["required"] = descriptor.InputSchema.Required
		}
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": descriptor.Name, "description": descriptor.Description, "parameters": parameters,
		}})
	}
	return tools
}

func (c *sentinelNativeConversation) next(ctx context.Context, remaining int) (sentinelAssistantTurn, string, int, error) {
	payload := map[string]any{"model": c.route.Model, "messages": c.messages, "tools": sentinelNativeCatalog()}
	if remaining <= 0 {
		payload["tool_choice"] = "none"
	}
	start := time.Now()
	reply, err := requestAICompletion(ctx, c.route.BaseURL, c.route.APIKey, payload)
	if err != nil {
		return sentinelAssistantTurn{}, c.route.Model, 0, fmt.Errorf("native model request failed: %w", err)
	}
	turn, err := c.normalize(reply, remaining)
	logSentinelNativeTurn(c.route, reply, time.Since(start), len(turn.ToolCalls), err != nil)
	return turn, c.route.Model, reply.Usage.TotalTokens, err
}

func (c *sentinelNativeConversation) normalize(reply aiCompletion, remaining int) (sentinelAssistantTurn, error) {
	choice := reply.Choices[0]
	if choice.FinishReason == "length" || choice.FinishReason == "content_filter" {
		return sentinelAssistantTurn{}, fmt.Errorf("native model did not complete its turn")
	}
	if len(choice.Message.ToolCalls) == 0 {
		turn, err := parseSentinelEnvelope(choice.Message.Content)
		if err == nil && len(turn.ToolCalls) != 0 {
			return sentinelAssistantTurn{}, fmt.Errorf("native model returned textual tool calls")
		}
		return turn, err
	}
	if len(choice.Message.ToolCalls) > remaining {
		return sentinelAssistantTurn{}, fmt.Errorf("native turn exceeds remaining tool budget")
	}
	turn := sentinelAssistantTurn{}
	seen := make(map[string]bool)
	for _, call := range choice.Message.ToolCalls {
		if call.Type != "function" || !sentinelIdentifierPattern.MatchString(call.ID) || seen[call.ID] || c.callIDs[call.ID] {
			return sentinelAssistantTurn{}, fmt.Errorf("native turn has invalid or repeated tool call IDs")
		}
		arguments := json.RawMessage(call.Function.Arguments)
		if trimmed := strings.TrimSpace(string(arguments)); !strings.HasPrefix(trimmed, "{") || !json.Valid(arguments) {
			return sentinelAssistantTurn{}, fmt.Errorf("native turn has invalid JSON arguments")
		}
		seen[call.ID] = true
		turn.ToolCalls = append(turn.ToolCalls, SentinelToolCall{Name: call.Function.Name, Arguments: arguments})
	}
	for id := range seen {
		c.callIDs[id] = true
	}
	choice.Message.Role = "assistant"
	choice.Message.Content = redactSentinelSecrets(choice.Message.Content)
	c.pending = choice.Message.ToolCalls
	c.messages = append(c.messages, choice.Message)
	return turn, nil
}

func (c *sentinelNativeConversation) addResult(index int, name, data string, success bool) {
	call := c.pending[index]
	body, _ := json.Marshal(map[string]any{
		"tool": name, "data_only": true, "trust_class": "untrusted_evidence",
		"ok": success, "data": data,
	})
	c.messages = append(c.messages, aiChatMessage{Role: "tool", ToolCallID: call.ID, Content: string(body)})
}

func logSentinelNativeTurn(route SentinelModelRoute, reply aiCompletion, duration time.Duration, calls int, failed bool) {
	slog.Info("Sentinel model turn", "route", route.Class, "model", route.Model, "mode", route.ToolMode,
		"latency_ms", duration.Milliseconds(), "tokens", reply.Usage.TotalTokens, "input_tokens", reply.Usage.PromptTokens, "output_tokens", reply.Usage.CompletionTokens, "calls", calls, "protocol_error", failed)
}
