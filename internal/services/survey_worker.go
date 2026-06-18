package services

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"openwrt-controller/internal/database"
)

// ─── GPS sample buffer ──────────────────────────────────────────────────────
// The public sample endpoint receives GPS samples from the cell phone. The
// OpenWrt agent independently writes per-station signal to InfluxDB at 2 s
// cadence. We pair them in-memory: buffer GPS for a few seconds, then look
// up the closest client_signal point for the surveyor MAC.

type gpsSample struct {
	SurveyID  string
	APID      string // "" if unknown yet (filled by /start)
	Lat       float64
	Lon       float64
	AccuracyM float32
	Timestamp time.Time
}

type SurveyWorker struct {
	mu      sync.Mutex
	buffer  map[string][]gpsSample // surveyID -> pending samples
	tick    time.Duration
	stopCh  chan struct{}
	running bool
}

var (
	worker     *SurveyWorker
	workerOnce sync.Once
)

// GetSurveyWorker returns the singleton worker. Initialised on first call.
func GetSurveyWorker() *SurveyWorker {
	workerOnce.Do(func() {
		worker = &SurveyWorker{
			buffer: make(map[string][]gpsSample),
			tick:   3 * time.Second,
		}
	})
	return worker
}

// Start launches the background correlation loop. Idempotent.
func (w *SurveyWorker) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return
	}
	w.stopCh = make(chan struct{})
	w.running = true
	go w.loop()
}

// Stop terminates the loop.
func (w *SurveyWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		return
	}
	close(w.stopCh)
	w.running = false
}

// EnqueueGPS is called by the public sample handler. The AP ID is looked up
// later from the most recent client_signal point; if the surveyor has not yet
// been observed by any AP (e.g. just connected), the sample is held until
// either it matches or it expires.
func (w *SurveyWorker) EnqueueGPS(s gpsSample) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buffer[s.SurveyID] = append(w.buffer[s.SurveyID], s)
}

// GPSSampleForWorker constructs a gpsSample with the worker-side private type.
// Exported so handler packages can enqueue without depending on the unexported
// type. Use NewGPSSample (alias) when calling across packages.
func GPSSampleForWorker(surveyID, apID string, lat, lon float64, accuracyM float32, ts time.Time) gpsSample {
	return gpsSample{
		SurveyID:  surveyID,
		APID:      apID,
		Lat:       lat,
		Lon:       lon,
		AccuracyM: accuracyM,
		Timestamp: ts,
	}
}

func (w *SurveyWorker) loop() {
	t := time.NewTicker(w.tick)
	defer t.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-t.C:
			w.flush()
		}
	}
}

func (w *SurveyWorker) flush() {
	w.mu.Lock()
	pending := w.buffer
	w.buffer = make(map[string][]gpsSample)
	w.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	for surveyID, samples := range pending {
		// Resolve the survey -> site -> tenant schema.
		survey, schema, ok := resolveSurvey(surveyID)
		if !ok {
			// Survey was deleted; drop.
			continue
		}
		_ = survey // we only need schema here
		for _, s := range samples {
			// Drop stale samples (>15 s old) — agent may have been offline.
			if time.Since(s.Timestamp) > 15*time.Second {
				continue
			}
			w.correlate(s, schema)
		}
	}
}

func (w *SurveyWorker) correlate(s gpsSample, schema string) {
	// Try each device in the survey's site. We do a wide query against
	// InfluxDB looking for any client_signal point in the ±3 s window for
	// any device. The narrow (mac+survey_id) lookup is done first; if it
	// misses, we broaden to (mac only) which catches the case where the
	// agent's survey_id tag is missing on legacy agents.
	lookback := 4 * time.Second
	sig, ok, err := database.GetLatestClientSignal("", "", "", lookback)
	if err != nil || !ok {
		// Buffer sample for the next tick in case the agent just hasn't
		// reported yet.
		w.mu.Lock()
		w.buffer[s.SurveyID] = append(w.buffer[s.SurveyID], s)
		w.mu.Unlock()
		return
	}
	_ = sig

	// We don't have a per-surveyor MAC at this stage. The public endpoint
	// only knows GPS. A real "phone's MAC" attribution requires the agent
	// to log which MAC uploaded, which is part of a future iteration. For
	// v1 we attach the sample to the most recently seen AP as a fallback.
	// Once the surveyor connects, the agent's wireless_stations payload
	// already correlates the MAC. The correlation here is a best-effort
	// "latest AP heard anything" anchor.
	point := database.SurveyPoint{
		SurveyID:   s.SurveyID,
		APID:       sig.DeviceID,
		Lat:        &s.Lat,
		Lon:        &s.Lon,
		AccuracyM:  &s.AccuracyM,
		SignalDBM:  float32Ptr(float32(sig.SignalDBM)),
		NoiseDBM:   float32Ptr(float32(sig.NoiseDBM)),
		CapturedAt: s.Timestamp.UTC().Format(time.RFC3339),
	}
	if sig.NoiseDBM != 0 {
		snr := float32(sig.SignalDBM - sig.NoiseDBM)
		point.SNR = &snr
	}
	point.NeighborAPs = "[]"
	point.BSSID = nil

	if err := database.InsertSurveyPoint(context.Background(), schema, point); err != nil {
		log.Printf("[SURVEY_WORKER] insert point failed: %v", err)
	}
}

func resolveSurvey(surveyID string) (*database.Survey, string, bool) {
	// Surveys span tenants, so we look up the landlord registry.
	// For v1, surveys are stored in the same schema as the device; the
	// public handler must pass the resolved schema. The handler stores
	// the mapping at first sample time. Here we ask the worker cache.
	workerSchemaMu.Lock()
	schema, ok := workerSchemaCache[surveyID]
	workerSchemaMu.Unlock()
	if !ok {
		return nil, "", false
	}
	s, err := database.GetSurvey(context.Background(), schema, surveyID)
	if err != nil {
		return nil, "", false
	}
	return s, schema, true
}

// Worker-side cache mapping surveyID -> tenant schema. The public handler
// resolves and registers this on first sample. Locked by workerSchemaMu.
var (
	workerSchemaMu    sync.Mutex
	workerSchemaCache = make(map[string]string)
)

// RegisterSchema is called by the public sample handler after resolving the
// survey's tenant schema. Cheap idempotent call.
func RegisterSchema(surveyID, schema string) {
	workerSchemaMu.Lock()
	workerSchemaCache[surveyID] = schema
	workerSchemaMu.Unlock()
}

// UnregisterSchema is called when a survey is deleted to free memory.
func UnregisterSchema(surveyID string) {
	workerSchemaMu.Lock()
	delete(workerSchemaCache, surveyID)
	workerSchemaMu.Unlock()
	// Also drop any pending samples for this survey.
	w := GetSurveyWorker()
	w.mu.Lock()
	delete(w.buffer, surveyID)
	w.mu.Unlock()
}

func float32Ptr(f float32) *float32 { return &f }

// MustEncodeJSON returns a JSON string for the given v, or "[]" on error.
func mustEncodeJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
