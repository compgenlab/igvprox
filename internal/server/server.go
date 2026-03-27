package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/compgenlab/igvprox/internal/config"
	"github.com/compgenlab/igvprox/internal/discovery"
)

//go:embed static/index.html
var staticFS embed.FS

type Options struct {
	Genome         string
	BrowserURL     string
	Files          []discovery.File
	ConstantTracks []config.Track
	Verbose        bool
}

type Server struct {
	genome         string
	files          []discovery.File
	constantTracks []config.Track
	byID           map[string]discovery.File
	mu             sync.RWMutex
	dynamicFiles   map[string]discovery.File
}

type sessionResponse struct {
	Genome   string         `json:"genome"`
	Hostname string         `json:"hostname"`
	CWD      string         `json:"cwd"`
	Tracks   []sessionTrack `json:"tracks"`
}

type sessionTrack struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path,omitempty"`
	Format         string `json:"format"`
	Type           string `json:"type,omitempty"`
	URL            string `json:"url"`
	IndexURL       string `json:"indexURL,omitempty"`
	DefaultEnabled bool   `json:"defaultEnabled"`
	Source         string `json:"source"`
}

type browseEntry struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"isDir"`
	Format   string `json:"format,omitempty"`
	HasIndex bool   `json:"hasIndex,omitempty"`
}

type browseResponse struct {
	Path    string        `json:"path"`
	Entries []browseEntry `json:"entries"`
}

type addTrackRequest struct {
	Path string `json:"path"`
}

func New(opts Options) *Server {
	byID := make(map[string]discovery.File, len(opts.Files))
	for _, file := range opts.Files {
		byID[file.ID] = file
	}
	return &Server{
		genome:         opts.Genome,
		files:          opts.Files,
		constantTracks: opts.ConstantTracks,
		byID:           byID,
		dynamicFiles:   make(map[string]discovery.File),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/browse", s.handleBrowse)
	mux.HandleFunc("/api/track", s.handleAddTrack)
	mux.HandleFunc("/files/", s.handleFile)
	return withLogging(mux)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "failed to load UI", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hostname, _ := os.Hostname()
	cwd, _ := os.Getwd()
	resp := sessionResponse{
		Genome:   s.genome,
		Hostname: hostname,
		CWD:      cwd,
		Tracks:   make([]sessionTrack, 0, len(s.files)+len(s.constantTracks)),
	}
	for _, file := range s.files {
		relPath := file.Path
		if cwd != "" {
			if rel, err := filepath.Rel(cwd, file.Path); err == nil {
				relPath = rel
			}
		}
		track := sessionTrack{
			ID:             file.ID,
			Name:           file.Name,
			Path:           relPath,
			Format:         file.Format,
			Type:           file.TrackType,
			URL:            fmt.Sprintf("/files/%s/data", file.ID),
			DefaultEnabled: true,
			Source:         "session",
		}
		if file.IndexPath != "" {
			track.IndexURL = fmt.Sprintf("/files/%s/index", file.ID)
		}
		resp.Tracks = append(resp.Tracks, track)
	}
	for _, track := range s.constantTracks {
		if track.Genome != "" && track.Genome != s.genome {
			continue
		}
		resp.Tracks = append(resp.Tracks, sessionTrack{
			ID:             config.TrackID(track),
			Name:           track.Name,
			Format:         track.Format,
			Type:           track.Type,
			URL:            track.URL,
			IndexURL:       track.IndexURL,
			DefaultEnabled: track.Enabled,
			Source:         "config",
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/files/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	kind := parts[1]

	s.mu.RLock()
	file, ok := s.byID[id]
	if !ok {
		file, ok = s.dynamicFiles[id]
	}
	s.mu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	switch kind {
	case "data":
		servePath(w, r, file.Path)
	case "index":
		if file.IndexPath == "" {
			http.NotFound(w, r)
			return
		}
		servePath(w, r, file.IndexPath)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			http.Error(w, "failed to get working directory", http.StatusInternalServerError)
			return
		}
	}

	path = filepath.Clean(path)

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		http.Error(w, "not a directory", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		http.Error(w, "failed to read directory", http.StatusInternalServerError)
		return
	}

	result := make([]browseEntry, 0, len(entries)+1)

	// Add parent entry unless at filesystem root
	parent := filepath.Dir(path)
	if parent != path {
		result = append(result, browseEntry{
			Name:  "..",
			Path:  parent,
			IsDir: true,
		})
	}

	// Directories first (os.ReadDir is already sorted by name)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			result = append(result, browseEntry{
				Name:  entry.Name(),
				Path:  filepath.Join(path, entry.Name()),
				IsDir: true,
			})
		}
	}

	// Then IGV-supported files
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || entry.IsDir() {
			continue
		}
		entryPath := filepath.Join(path, entry.Name())
		format, hasIndex := igvFileFormat(entryPath)
		if format == "" {
			continue
		}
		result = append(result, browseEntry{
			ID:       discovery.FileID(entryPath),
			Name:     entry.Name(),
			Path:     entryPath,
			IsDir:    false,
			Format:   format,
			HasIndex: hasIndex,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(browseResponse{Path: path, Entries: result})
}

func (s *Server) handleAddTrack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req addTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	files, _, err := discovery.Collect([]string{req.Path}, discovery.Options{AllowMissingIndex: true})
	if err != nil || len(files) == 0 {
		http.Error(w, "failed to collect file", http.StatusBadRequest)
		return
	}
	file := files[0]

	s.mu.Lock()
	if _, ok := s.byID[file.ID]; !ok {
		if _, ok := s.dynamicFiles[file.ID]; !ok {
			s.dynamicFiles[file.ID] = file
		}
	}
	s.mu.Unlock()

	cwd, _ := os.Getwd()
	relPath := file.Path
	if cwd != "" {
		if rel, err := filepath.Rel(cwd, file.Path); err == nil {
			relPath = rel
		}
	}

	track := sessionTrack{
		ID:             file.ID,
		Name:           file.Name,
		Path:           relPath,
		Format:         file.Format,
		Type:           file.TrackType,
		URL:            fmt.Sprintf("/files/%s/data", file.ID),
		DefaultEnabled: true,
		Source:         "browse",
	}
	if file.IndexPath != "" {
		track.IndexURL = fmt.Sprintf("/files/%s/index", file.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(track)
}

func igvFileFormat(path string) (format string, hasIndex bool) {
	lower := strings.ToLower(path)

	// Skip index files
	if strings.HasSuffix(lower, ".bai") || strings.HasSuffix(lower, ".crai") ||
		strings.HasSuffix(lower, ".tbi") || strings.HasSuffix(lower, ".csi") {
		return "", false
	}

	checkIndex := func(candidates ...string) bool {
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return true
			}
		}
		return false
	}

	switch {
	case strings.HasSuffix(lower, ".bam"):
		return "bam", checkIndex(path+".bai", strings.TrimSuffix(path, ".bam")+".bai")
	case strings.HasSuffix(lower, ".cram"):
		return "cram", checkIndex(path + ".crai")
	case strings.HasSuffix(lower, ".vcf.gz"):
		return "vcf", checkIndex(path+".tbi", path+".csi")
	case strings.HasSuffix(lower, ".bed.gz"):
		return "bed", checkIndex(path+".tbi", path+".csi")
	case strings.HasSuffix(lower, ".bedgraph.gz"), strings.HasSuffix(lower, ".bg.gz"):
		return "bedgraph", checkIndex(path+".tbi", path+".csi")
	case strings.HasSuffix(lower, ".bigwig"), strings.HasSuffix(lower, ".bw"):
		return "bigwig", false
	case strings.HasSuffix(lower, ".bigbed"), strings.HasSuffix(lower, ".bb"):
		return "bigbed", false
	case strings.HasSuffix(lower, ".bed"):
		return "bed", false
	case strings.HasSuffix(lower, ".sam"):
		return "sam", false
	default:
		return "", false
	}
}

func servePath(w http.ResponseWriter, r *http.Request, path string) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(w, "failed to stat file", http.StatusInternalServerError)
		return
	}

	if contentType := detectContentType(path); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

func detectContentType(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".bam"):
		return "application/octet-stream"
	case strings.HasSuffix(lower, ".bai"):
		return "application/octet-stream"
	case strings.HasSuffix(lower, ".cram"):
		return "application/octet-stream"
	case strings.HasSuffix(lower, ".crai"):
		return "application/octet-stream"
	case strings.HasSuffix(lower, ".vcf.gz"):
		return "application/gzip"
	case strings.HasSuffix(lower, ".tbi"), strings.HasSuffix(lower, ".csi"):
		return "application/octet-stream"
	case strings.HasSuffix(lower, ".bed.gz"), strings.HasSuffix(lower, ".bedgraph.gz"), strings.HasSuffix(lower, ".bg.gz"):
		return "application/gzip"
	default:
		return ""
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
