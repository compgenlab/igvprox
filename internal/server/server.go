package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/compgenlab/igvprox/internal/discovery"
)

//go:embed static/index.html
var staticFS embed.FS

type Options struct {
	Genome     string
	BrowserURL string
	Files      []discovery.File
	Verbose    bool
}

type Server struct {
	genome string
	files  []discovery.File
	byID   map[string]discovery.File
}

type sessionResponse struct {
	Genome string         `json:"genome"`
	Tracks []sessionTrack `json:"tracks"`
}

type sessionTrack struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Format   string `json:"format"`
	Type     string `json:"type,omitempty"`
	URL      string `json:"url"`
	IndexURL string `json:"indexURL,omitempty"`
}

func New(opts Options) *Server {
	byID := make(map[string]discovery.File, len(opts.Files))
	for _, file := range opts.Files {
		byID[file.ID] = file
	}
	return &Server{
		genome: opts.Genome,
		files:  opts.Files,
		byID:   byID,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/session", s.handleSession)
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
	_, _ = w.Write(data)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := sessionResponse{
		Genome: s.genome,
		Tracks: make([]sessionTrack, 0, len(s.files)),
	}
	for _, file := range s.files {
		track := sessionTrack{
			Name:   file.Name,
			Path:   file.Path,
			Format: file.Format,
			Type:   file.TrackType,
			URL:    fmt.Sprintf("/files/%s/data", file.ID),
		}
		if file.IndexPath != "" {
			track.IndexURL = fmt.Sprintf("/files/%s/index", file.ID)
		}
		resp.Tracks = append(resp.Tracks, track)
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
	file, ok := s.byID[id]
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
