package services

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

var sentinelHostapdEvent = regexp.MustCompile(`(?i)^(?:[A-Z][a-z]{2} [A-Z][a-z]{2} +[0-9]{1,2} [0-9]{2}:[0-9]{2}:[0-9]{2} [0-9]{4} daemon\.[a-z]+ )?hostapd: ([a-z0-9_.-]+): (?:AP-STA-CONNECTED ((?:[0-9a-f]{2}:){5}[0-9a-f]{2})(?: auth_alg=([a-z0-9_-]+))?|STA ((?:[0-9a-f]{2}:){5}[0-9a-f]{2}) IEEE 802\.11: association rejected(?: \([^\r\n]*\))?)$`)

type sentinelRoamingEvent struct {
	At                time.Time  `json:"observed_at"`
	Device            string     `json:"device_id"`
	Interface         string     `json:"interface"`
	Station           string     `json:"station"`
	Outcome           string     `json:"outcome"`
	FollowupAt        *time.Time `json:"followup_at,omitempty"`
	TrafficContinuity string     `json:"traffic_continuity"`
}

func sentinelRoamingQuery(query string) bool {
	for _, term := range []string{"roaming", "handoff", "handover", "traspaso", "itinerancia", "802 11r", "ft"} {
		if sentinelQueryContainsTerm(query, term) {
			return true
		}
	}
	return false
}

// This is a derived view of untrusted AP observations. An association event
// alone cannot establish what the client observed on its data path.
func sentinelRoamingEvidence(tool, encoded string) string {
	if tool != "search_logs" {
		return encoded
	}
	var logs []struct {
		At      time.Time `json:"timestamp"`
		Device  string    `json:"device_id"`
		Message string    `json:"message"`
	}
	if err := json.Unmarshal([]byte(encoded), &logs); err != nil {
		return encoded
	}
	var events []sentinelRoamingEvent
	for _, log := range logs {
		match := sentinelHostapdEvent.FindStringSubmatch(strings.TrimSpace(log.Message))
		if match == nil || log.At.IsZero() || log.Device == "" {
			continue
		}
		event := sentinelRoamingEvent{
			At: log.At, Device: log.Device, Interface: match[1],
			Station: strings.ToLower(match[2]), Outcome: "association_connected", TrafficContinuity: "unknown",
		}
		if match[4] != "" {
			event.Station = strings.ToLower(match[4])
			event.Outcome = "association_rejected"
		} else if strings.EqualFold(match[3], "ft") {
			event.Outcome = "ft_connected"
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return encoded
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].At.Before(events[j].At) })
	for i := range events {
		event := &events[i]
		if event.Outcome != "association_rejected" {
			continue
		}
		for _, next := range events[i+1:] {
			elapsed := next.At.Sub(event.At)
			if elapsed > 10*time.Second {
				break
			}
			if elapsed > 0 && next.Station == event.Station && next.Device == event.Device && next.Interface == event.Interface {
				if next.Outcome != "association_rejected" {
					event.FollowupAt = &next.At
				}
				break
			}
		}
	}
	if len(events) > 12 {
		events = events[len(events)-12:]
	}
	wrapped, err := json.Marshal(struct {
		Events       []sentinelRoamingEvent `json:"roaming_events"`
		Observations json.RawMessage        `json:"observations"`
	}{Events: events, Observations: json.RawMessage(encoded)})
	if err != nil {
		return encoded
	}
	return string(wrapped)
}
