package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"openwrt-controller/internal/database"
)

const (
	maxKMZUploadBytes = 128 << 20 // 128 MiB
	maxKMZTileBytes   = 8 << 20   // 8 MiB per single tile safety cap
	maxKMZSingleBytes = 32 << 20  // 32 MiB for single-image GroundOverlay
)

var (
	kmzSiteIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	kmzTileRe   = regexp.MustCompile(`^(\d+)/(\d+)/(\d+)\.(png|jpg|jpeg|webp)$`)
	kmlTagRe    = map[string]*regexp.Regexp{
		"name":  regexp.MustCompile(`(?is)<name>\s*([^<]+?)\s*</name>`),
		"north": regexp.MustCompile(`(?is)<north>\s*([-+]?\d+(?:\.\d+)?)\s*</north>`),
		"south": regexp.MustCompile(`(?is)<south>\s*([-+]?\d+(?:\.\d+)?)\s*</south>`),
		"east":  regexp.MustCompile(`(?is)<east>\s*([-+]?\d+(?:\.\d+)?)\s*</east>`),
		"west":  regexp.MustCompile(`(?is)<west>\s*([-+]?\d+(?:\.\d+)?)\s*</west>`),
		"href":  regexp.MustCompile(`(?is)<GroundOverlay\b.*?<Icon\b.*?<href>\s*([^<]+?)\s*</href>`),
		"quad":  regexp.MustCompile(`(?is)<(?:gx:)?LatLonQuad\b.*?<coordinates>\s*([^<]+?)\s*</coordinates>`),
	}
)

type SiteKMZMeta struct {
	Name        string        `json:"name"`
	Filename    string        `json:"filename"`
	Mode        string        `json:"mode"` // "superoverlay" | "single"
	Bounds      [2][2]float64 `json:"bounds"`
	MinLevel    int           `json:"min_level"`
	MaxLevel    int           `json:"max_level"`
	TileCount   int           `json:"tile_count"`
	SingleEntry string        `json:"single_entry,omitempty"`
	SizeBytes   int64         `json:"size_bytes"`
	UpdatedAt   string        `json:"updated_at"`
}

type parsedKMZ struct {
	Meta        SiteKMZMeta
	TileIndex   map[string]string // "z/x/y" -> zip entry path
	SingleEntry string
}

type cachedKMZ struct {
	meta        SiteKMZMeta
	zipPath     string
	modTime     time.Time
	reader      *zip.ReadCloser
	entryByName map[string]*zip.File
	tileIndex   map[string]string
	singleEntry string
}

var (
	kmzCacheMu sync.RWMutex
	kmzCache   = make(map[string]*cachedKMZ)
)

func surveyMapsBaseDir() string {
	if v := strings.TrimSpace(os.Getenv("SURVEY_MAPS_DIR")); v != "" {
		return v
	}
	return filepath.Join("data", "site-maps")
}

func validateKMZSiteID(siteID string) bool {
	return kmzSiteIDRe.MatchString(siteID)
}

func siteKMZPaths(siteID string) (dir, kmzPath, metaPath string) {
	dir = filepath.Join(surveyMapsBaseDir(), strings.ToLower(siteID))
	kmzPath = filepath.Join(dir, "ortho.kmz")
	metaPath = filepath.Join(dir, "meta.json")
	return
}

func extractKMLBounds(kml string) ([2][2]float64, bool) {
	var b [2][2]float64
	nM := kmlTagRe["north"].FindStringSubmatch(kml)
	sM := kmlTagRe["south"].FindStringSubmatch(kml)
	eM := kmlTagRe["east"].FindStringSubmatch(kml)
	wM := kmlTagRe["west"].FindStringSubmatch(kml)
	if len(nM) == 2 && len(sM) == 2 && len(eM) == 2 && len(wM) == 2 {
		north, err1 := strconv.ParseFloat(nM[1], 64)
		south, err2 := strconv.ParseFloat(sM[1], 64)
		east, err3 := strconv.ParseFloat(eM[1], 64)
		west, err4 := strconv.ParseFloat(wM[1], 64)
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil &&
			south >= -90 && north <= 90 && west >= -180 && east <= 180 &&
			south < north && west < east {
			b = [2][2]float64{{south, west}, {north, east}}
			return b, true
		}
	}
	if qM := kmlTagRe["quad"].FindStringSubmatch(kml); len(qM) == 2 {
		fields := strings.Fields(qM[1])
		minLat, maxLat := 91.0, -91.0
		minLon, maxLon := 181.0, -181.0
		count := 0
		for _, tok := range fields {
			parts := strings.Split(tok, ",")
			if len(parts) < 2 {
				continue
			}
			lon, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lat, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err1 != nil || err2 != nil {
				continue
			}
			if lat < minLat {
				minLat = lat
			}
			if lat > maxLat {
				maxLat = lat
			}
			if lon < minLon {
				minLon = lon
			}
			if lon > maxLon {
				maxLon = lon
			}
			count++
		}
		if count >= 3 && minLat < maxLat && minLon < maxLon &&
			minLat >= -90 && maxLat <= 90 && minLon >= -180 && maxLon <= 180 {
			return [2][2]float64{{minLat, minLon}, {maxLat, maxLon}}, true
		}
	}
	return b, false
}

func readZipTextLimited(f *zip.File, maxBytes int64) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, maxBytes))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseKMZArchive(path, filename string) (*parsedKMZ, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("invalid KMZ archive: %w", err)
	}
	defer zr.Close()

	var docKMLFile, rootTileKMLFile *zip.File
	filesByName := make(map[string]*zip.File, len(zr.File))
	tileIndex := make(map[string]string)
	minLevel, maxLevel := 999, -1

	for _, f := range zr.File {
		clean := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		filesByName[clean] = f
		lower := strings.ToLower(clean)
		if lower == "doc.kml" {
			docKMLFile = f
		} else if lower == "0/0/0.kml" {
			rootTileKMLFile = f
		} else if docKMLFile == nil && strings.HasSuffix(lower, ".kml") && !strings.Contains(clean, "/") {
			docKMLFile = f
		}
		if m := kmzTileRe.FindStringSubmatch(lower); len(m) == 5 {
			lvl, err := strconv.Atoi(m[1])
			if err == nil {
				if lvl < minLevel {
					minLevel = lvl
				}
				if lvl > maxLevel {
					maxLevel = lvl
				}
				key := m[1] + "/" + m[2] + "/" + m[3]
				tileIndex[key] = clean
			}
		}
	}

	var docKML, rootKML string
	if docKMLFile != nil {
		docKML, _ = readZipTextLimited(docKMLFile, 1<<20)
	}
	if rootTileKMLFile != nil {
		rootKML, _ = readZipTextLimited(rootTileKMLFile, 1<<20)
	}
	if docKML == "" && rootKML == "" {
		return nil, fmt.Errorf("KMZ does not contain a readable doc.kml or 0/0/0.kml")
	}

	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if m := kmlTagRe["name"].FindStringSubmatch(docKML); len(m) == 2 && strings.TrimSpace(m[1]) != "" {
		name = strings.TrimSpace(m[1])
	}

	// Prefer 0/0/0.kml bounds for superoverlays because it matches the exact tile grid.
	var bounds [2][2]float64
	var ok bool
	if len(tileIndex) > 0 && rootKML != "" {
		bounds, ok = extractKMLBounds(rootKML)
	}
	if !ok && docKML != "" {
		bounds, ok = extractKMLBounds(docKML)
	}
	if !ok && rootKML != "" {
		bounds, ok = extractKMLBounds(rootKML)
	}
	if !ok {
		return nil, fmt.Errorf("could not extract valid geographic bounds from KMZ")
	}

	meta := SiteKMZMeta{
		Name:      name,
		Filename:  filepath.Base(filename),
		Bounds:    bounds,
		SizeBytes: st.Size(),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if len(tileIndex) > 0 {
		meta.Mode = "superoverlay"
		meta.MinLevel = minLevel
		meta.MaxLevel = maxLevel
		meta.TileCount = len(tileIndex)
		return &parsedKMZ{Meta: meta, TileIndex: tileIndex}, nil
	}

	// Single-image GroundOverlay fallback
	hrefMatch := kmlTagRe["href"].FindStringSubmatch(docKML)
	if len(hrefMatch) != 2 {
		return nil, fmt.Errorf("KMZ has neither SuperOverlay tiles nor a GroundOverlay image")
	}
	href := strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(hrefMatch[1]), "\\", "/"), "./")
	if _, exists := filesByName[href]; !exists {
		return nil, fmt.Errorf("GroundOverlay image %q not found inside KMZ", href)
	}
	ext := strings.ToLower(filepath.Ext(href))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
		return nil, fmt.Errorf("unsupported GroundOverlay image format %q", ext)
	}
	meta.Mode = "single"
	meta.TileCount = 1
	meta.SingleEntry = href
	return &parsedKMZ{Meta: meta, SingleEntry: href}, nil
}

func evictKMZCache(siteID string) {
	key := strings.ToLower(siteID)
	kmzCacheMu.Lock()
	if prev, ok := kmzCache[key]; ok {
		if prev.reader != nil {
			_ = prev.reader.Close()
		}
		delete(kmzCache, key)
	}
	kmzCacheMu.Unlock()
}

func installSiteKMZ(siteID, srcPath, filename string) (*SiteKMZMeta, error) {
	if !validateKMZSiteID(siteID) {
		return nil, fmt.Errorf("invalid site_id")
	}
	parsed, err := parseKMZArchive(srcPath, filename)
	if err != nil {
		return nil, err
	}
	dir, kmzPath, metaPath := siteKMZPaths(siteID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}

	tmpKMZ := kmzPath + ".tmp"
	in, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	out, err := os.OpenFile(tmpKMZ, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		in.Close()
		return nil, err
	}
	if _, err := io.Copy(out, in); err != nil {
		in.Close()
		out.Close()
		_ = os.Remove(tmpKMZ)
		return nil, err
	}
	in.Close()
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpKMZ)
		return nil, err
	}

	metaBytes, err := json.MarshalIndent(parsed.Meta, "", "  ")
	if err != nil {
		_ = os.Remove(tmpKMZ)
		return nil, err
	}
	tmpMeta := metaPath + ".tmp"
	if err := os.WriteFile(tmpMeta, metaBytes, 0o640); err != nil {
		_ = os.Remove(tmpKMZ)
		return nil, err
	}

	evictKMZCache(siteID)
	if err := os.Rename(tmpKMZ, kmzPath); err != nil {
		_ = os.Remove(tmpKMZ)
		_ = os.Remove(tmpMeta)
		return nil, err
	}
	if err := os.Rename(tmpMeta, metaPath); err != nil {
		_ = os.Remove(tmpMeta)
		return nil, err
	}
	return &parsed.Meta, nil
}

func loadSiteKMZCached(siteID string) (*cachedKMZ, error) {
	if !validateKMZSiteID(siteID) {
		return nil, fmt.Errorf("invalid site_id")
	}
	_, kmzPath, metaPath := siteKMZPaths(siteID)
	st, err := os.Stat(kmzPath)
	if err != nil {
		return nil, err
	}
	key := strings.ToLower(siteID)
	kmzCacheMu.RLock()
	if c, ok := kmzCache[key]; ok && c.zipPath == kmzPath && c.modTime.Equal(st.ModTime()) {
		kmzCacheMu.RUnlock()
		return c, nil
	}
	kmzCacheMu.RUnlock()

	var meta SiteKMZMeta
	metaRaw, err := os.ReadFile(metaPath)
	if err == nil {
		_ = json.Unmarshal(metaRaw, &meta)
	}
	parsed, err := parseKMZArchive(kmzPath, meta.Filename)
	if err != nil {
		return nil, err
	}
	if meta.Name != "" {
		parsed.Meta.Name = meta.Name
	}
	if meta.Filename != "" {
		parsed.Meta.Filename = meta.Filename
	}
	if meta.UpdatedAt != "" {
		parsed.Meta.UpdatedAt = meta.UpdatedAt
	}

	zr, err := zip.OpenReader(kmzPath)
	if err != nil {
		return nil, err
	}
	entries := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		clean := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		entries[clean] = f
	}

	entry := &cachedKMZ{
		meta:        parsed.Meta,
		zipPath:     kmzPath,
		modTime:     st.ModTime(),
		reader:      zr,
		entryByName: entries,
		tileIndex:   parsed.TileIndex,
		singleEntry: parsed.SingleEntry,
	}

	kmzCacheMu.Lock()
	if prev, ok := kmzCache[key]; ok && prev.reader != nil {
		_ = prev.reader.Close()
	}
	kmzCache[key] = entry
	kmzCacheMu.Unlock()
	return entry, nil
}

func mimeForImageExt(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// ─── HTTP Handlers ──────────────────────────────────────────────────────────

func GetSiteKMZMetaHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if !validateKMZSiteID(siteID) {
		http.Error(w, `{"error":"invalid site_id"}`, http.StatusBadRequest)
		return
	}
	c, err := loadSiteKMZCached(siteID)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"data": nil})
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to read KMZ metadata", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": c.meta})
}

func UploadSiteKMZHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if !validateKMZSiteID(siteID) {
		http.Error(w, `{"error":"invalid site_id"}`, http.StatusBadRequest)
		return
	}
	username, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") {
		http.Error(w, `{"error":"FORBIDDEN: OPERATOR+ required"}`, http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxKMZUploadBytes)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		http.Error(w, `{"error":"invalid multipart upload or file exceeds 128 MB limit"}`, http.StatusBadRequest)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"missing 'file' form field"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmpFile, err := os.CreateTemp("", "kmz-upload-*.kmz")
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create temp file", err)
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		RespondError(w, http.StatusInternalServerError, "failed to read upload", err)
		return
	}
	if err := tmpFile.Close(); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to flush upload", err)
		return
	}

	filename := "orthophoto.kmz"
	if header != nil && header.Filename != "" {
		filename = header.Filename
	}
	meta, err := installSiteKMZ(siteID, tmpPath, filename)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	go database.InsertAuditLog(username, "SITE_KMZ_UPLOAD", "SITE", siteID, filename, r.RemoteAddr)
	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": meta})
}

func DeleteSiteKMZHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if !validateKMZSiteID(siteID) {
		http.Error(w, `{"error":"invalid site_id"}`, http.StatusBadRequest)
		return
	}
	username, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") {
		http.Error(w, `{"error":"FORBIDDEN: OPERATOR+ required"}`, http.StatusForbidden)
		return
	}
	evictKMZCache(siteID)
	_, kmzPath, metaPath := siteKMZPaths(siteID)
	_ = os.Remove(kmzPath)
	_ = os.Remove(metaPath)
	go database.InsertAuditLog(username, "SITE_KMZ_DELETE", "SITE", siteID, "", r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

func GetSiteKMZTileHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if !validateKMZSiteID(siteID) {
		http.Error(w, `{"error":"invalid site_id"}`, http.StatusBadRequest)
		return
	}
	z, err1 := strconv.Atoi(r.PathValue("z"))
	x, err2 := strconv.Atoi(r.PathValue("x"))
	y, err3 := strconv.Atoi(r.PathValue("y"))
	if err1 != nil || err2 != nil || err3 != nil || z < 0 || z > 20 || x < 0 || y < 0 {
		http.Error(w, `{"error":"invalid tile coordinates"}`, http.StatusBadRequest)
		return
	}

	c, err := loadSiteKMZCached(siteID)
	if err != nil {
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to open KMZ", err)
		return
	}
	key := fmt.Sprintf("%d/%d/%d", z, x, y)
	entryName, ok := c.tileIndex[key]
	if !ok {
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	zf := c.entryByName[entryName]
	if zf == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	rc, err := zf.Open()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to read tile", err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", mimeForImageExt(entryName))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = io.Copy(w, io.LimitReader(rc, maxKMZTileBytes))
}

func GetSiteKMZImageHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if !validateKMZSiteID(siteID) {
		http.Error(w, `{"error":"invalid site_id"}`, http.StatusBadRequest)
		return
	}
	c, err := loadSiteKMZCached(siteID)
	if err != nil || c.singleEntry == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	zf := c.entryByName[c.singleEntry]
	if zf == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	rc, err := zf.Open()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to read KMZ image", err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", mimeForImageExt(c.singleEntry))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = io.Copy(w, io.LimitReader(rc, maxKMZSingleBytes))
}
