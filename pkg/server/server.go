package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/aalsi-app/aalsi/pkg/digest"
	"github.com/aalsi-app/aalsi/pkg/extractor"
	"github.com/aalsi-app/aalsi/pkg/ics"
	"github.com/aalsi-app/aalsi/pkg/model"
	"github.com/aalsi-app/aalsi/pkg/parser"
	"github.com/aalsi-app/aalsi/pkg/prefilter"
	"github.com/aalsi-app/aalsi/pkg/store"
	"github.com/aalsi-app/aalsi/pkg/tracker"
	"github.com/aalsi-app/aalsi/web"
)

// Server encapsulates the HTTP service and dependencies
type Server struct {
	store     *store.Store
	tracker   *tracker.Engine
	extractor *extractor.Extractor
	mux       *http.ServeMux
	loc       *time.Location
	modelName string
}

// Config configures Server initialization
type Config struct {
	Store     *store.Store
	Tracker   *tracker.Engine
	Extractor *extractor.Extractor
	Location  *time.Location
	ModelName string
}

// NewServer builds and mounts the HTTP routes
func NewServer(cfg Config) *Server {
	if cfg.Location == nil {
		cfg.Location = time.FixedZone("IST", 5*3600+30*60)
	}
	if cfg.ModelName == "" {
		cfg.ModelName = "gemma3:12b"
	}

	s := &Server{
		store:     cfg.Store,
		tracker:   cfg.Tracker,
		extractor: cfg.Extractor,
		mux:       http.NewServeMux(),
		loc:       cfg.Location,
		modelName: cfg.ModelName,
	}

	s.routes()
	return s
}

func (s *Server) routes() {
	// Static frontend via embed
	s.mux.HandleFunc("/", s.handleRoot)
	s.mux.HandleFunc("/static/", s.handleStatic)

	// API endpoints
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/i18n", s.handleI18n)
	s.mux.HandleFunc("/api/upload", s.handleUpload)
	s.mux.HandleFunc("/api/digest", s.handleDigest)
	s.mux.HandleFunc("/api/digest/plain", s.handleDigestPlain)
	s.mux.HandleFunc("/api/export.ics", s.handleExportICS)
	s.mux.HandleFunc("/api/items", s.handleItems)
	s.mux.HandleFunc("/api/review/", s.handleReviewAction)
	s.mux.HandleFunc("/api/purge", s.handlePurge)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Structured logging that NEVER prints message text or sensitive user data
	start := time.Now()
	s.mux.ServeHTTP(w, r)
	log.Printf("[HTTP] method=%s path=%s duration_ms=%d", r.Method, r.URL.Path, time.Since(start).Milliseconds())
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := web.Assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Asset not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(r.URL.Path, "/static/")
	data, err := web.Assets.ReadFile(filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch {
	case strings.HasSuffix(filename, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(filename, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	}
	w.Write(data)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	ollamaOnline := false
	var installedModels []string
	modelInstalled := false

	targetModel := r.URL.Query().Get("model")
	if targetModel == "" {
		targetModel = s.modelName
	}

	if s.extractor != nil {
		models, err := s.extractor.GetInstalledModels(ctx)
		if err == nil {
			ollamaOnline = true
			installedModels = models
			for _, m := range models {
				if strings.HasPrefix(m, targetModel) {
					modelInstalled = true
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "ok",
		"ollama_online":    ollamaOnline,
		"model":            targetModel,
		"model_installed":  modelInstalled,
		"installed_models": installedModels,
		"timezone":         s.loc.String(),
	})
}

func (s *Server) handleI18n(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Translations)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		http.Error(w, "No files uploaded", http.StatusBadRequest)
		return
	}

	totalImported := 0
	ctx := r.Context()

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			continue
		}

		groupID := fileHeader.Filename
		if idx := strings.LastIndex(groupID, "."); idx != -1 {
			groupID = groupID[:idx]
		}

		parsed, err := parser.ParseChat(file, parser.ParseOptions{
			GroupID:  groupID,
			Location: s.loc,
		})
		file.Close()
		if err != nil {
			continue
		}

		// Store parsed messages idempotently
		inserted, err := s.store.ImportMessages(ctx, parsed.Messages)
		if err == nil {
			totalImported += inserted
		}

		// Prefilter for candidate messages
		candidates := prefilter.FilterMessages(parsed.Messages)

		// Create message map for tracker reference
		msgMap := make(map[string]model.Message)
		for _, m := range parsed.Messages {
			msgMap[m.ID] = m
		}

		// Check extraction mode: "auto" (default), "rule" (explicit), or "gemma"
		mode := r.FormValue("mode") // "auto", "rule", "gemma"

		// Extract events
		if len(candidates) > 0 {
			var events []model.RawExtractedEvent
			if mode == "rule" {
				events = extractor.FallbackRuleExtractor(candidates)
			} else {
				// Must try Gemma
				if s.extractor == nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error": "ollama_unreachable",
						"message": "Ollama is not running. Please start Ollama or choose 'Process in rule mode'.",
					})
					return
				}
				var err error
				events, err = s.extractor.ChunkAndExtract(ctx, candidates, 3500, 500)
				if err != nil {
					// Never silently fall back! If Ollama is unreachable, return error asking user
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error": "ollama_unreachable",
						"message": fmt.Sprintf("Ollama is unreachable (%v). You can process in rule mode or cancel.", err),
					})
					return
				}
			}

			for _, ev := range events {
				s.tracker.ProcessRawEvent(ctx, groupID, ev, msgMap)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":           true,
		"imported_messages": totalImported,
	})
}

func (s *Server) handleDigest(w http.ResponseWriter, r *http.Request) {
	lang := digest.Language(r.URL.Query().Get("lang"))
	items, err := s.store.GetItems(r.Context(), false)
	if err != nil {
		http.Error(w, "Failed to query items", http.StatusInternalServerError)
		return
	}

	d := digest.Generate(items, time.Now().In(s.loc), lang)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

func (s *Server) handleDigestPlain(w http.ResponseWriter, r *http.Request) {
	lang := digest.Language(r.URL.Query().Get("lang"))
	items, err := s.store.GetItems(r.Context(), false)
	if err != nil {
		http.Error(w, "Failed to query items", http.StatusInternalServerError)
		return
	}

	d := digest.Generate(items, time.Now().In(s.loc), lang)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(d.FormatPlainText()))
}

func (s *Server) handleExportICS(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.GetItems(r.Context(), false)
	if err != nil {
		http.Error(w, "Failed to query items", http.StatusInternalServerError)
		return
	}

	cal := ics.GenerateCalendar(items)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"aalsi_deadlines.ics\"")
	w.Write([]byte(cal))
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.GetItems(r.Context(), false)
	if err != nil {
		http.Error(w, "Failed to query items", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (s *Server) handleReviewAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// e.g. api/review/{id}/confirm or api/review/{id}/dismiss
	if len(parts) < 4 {
		http.Error(w, "Invalid review URL", http.StatusBadRequest)
		return
	}
	itemID := parts[2]
	action := parts[3]

	var status model.ItemStatus
	var needsReview bool

	switch action {
	case "confirm":
		status = model.StatusConfirmed
		needsReview = false
	case "dismiss":
		status = model.StatusDismissed
		needsReview = false
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateItemStatus(r.Context(), itemID, status, needsReview); err != nil {
		http.Error(w, "Failed to update item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handlePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.store.PurgeAll(r.Context()); err != nil {
		http.Error(w, "Failed to purge database: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// DetectLANIPv4 finds the primary outbound LAN IPv4 address
func DetectLANIPv4() (string, error) {
	// 1. Try dialing a public endpoint (UDP connect without sending packets)
	// This lets the OS routing table choose the true outbound LAN interface.
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		if localAddr.IP.To4() != nil && !localAddr.IP.IsLoopback() {
			return localAddr.IP.String(), nil
		}
	}

	// 2. Fallback to scanning interfaces if offline
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				// Prefer standard private IPv4 subnets (192.168.x.x, 10.x.x.x, 172.16-31.x.x)
				ipStr := ipnet.IP.String()
				if strings.HasPrefix(ipStr, "192.168.") || strings.HasPrefix(ipStr, "10.") || strings.HasPrefix(ipStr, "172.") {
					return ipStr, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no active LAN IPv4 address detected on machine")
}
