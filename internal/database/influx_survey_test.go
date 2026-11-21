package database

import (
	"testing"
	"time"
)

func TestSelectBestSurveySignalPrefersActiveOverStaleGhost(t *testing.T) {
	base := time.Date(2026, 11, 21, 17, 52, 11, 500_000_000, time.UTC)

	t.Run("active AP beats closer-in-time stale ghost AP", func(t *testing.T) {
		candidates := []ClientSignalSample{
			{DeviceID: "ap-2-ghost", SignalDBM: -64, InactiveMs: 11030, Time: base.Add(170 * time.Millisecond)},
			{DeviceID: "ap-1-active", SignalDBM: -76, InactiveMs: 10, Time: base.Add(2600 * time.Millisecond)},
		}
		got, ok := selectBestSurveySignal(candidates, base)
		if !ok {
			t.Fatal("expected match")
		}
		if got.DeviceID != "ap-1-active" {
			t.Fatalf("got %q, want ap-1-active", got.DeviceID)
		}
	})

	t.Run("fringe stale AP is still used when no active AP exists", func(t *testing.T) {
		candidates := []ClientSignalSample{
			{DeviceID: "ap-2-fringe", SignalDBM: -76, InactiveMs: 6650, Time: base.Add(1 * time.Second)},
		}
		got, ok := selectBestSurveySignal(candidates, base)
		if !ok {
			t.Fatal("expected fringe match")
		}
		if got.DeviceID != "ap-2-fringe" {
			t.Fatalf("got %q, want ap-2-fringe", got.DeviceID)
		}
	})

	t.Run("extreme ghost over 15s inactive is discarded", func(t *testing.T) {
		candidates := []ClientSignalSample{
			{DeviceID: "ap-2-zombie", SignalDBM: -64, InactiveMs: 24330, Time: base},
		}
		if _, ok := selectBestSurveySignal(candidates, base); ok {
			t.Fatal("expected zombie sample (>15s inactive) to be discarded")
		}
	})

	t.Run("among two active APs picks closest timestamp", func(t *testing.T) {
		candidates := []ClientSignalSample{
			{DeviceID: "ap-1", SignalDBM: -68, InactiveMs: 200, Time: base.Add(3 * time.Second)},
			{DeviceID: "ap-3", SignalDBM: -59, InactiveMs: 120, Time: base.Add(500 * time.Millisecond)},
		}
		got, ok := selectBestSurveySignal(candidates, base)
		if !ok || got.DeviceID != "ap-3" {
			t.Fatalf("got (%v, %t), want ap-3", got.DeviceID, ok)
		}
	})
}
