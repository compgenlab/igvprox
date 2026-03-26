package discovery

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Options struct {
	Recursive         bool
	AllowMissingIndex bool
}

type File struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Format    string `json:"format"`
	TrackType string `json:"trackType"`
	IndexPath string `json:"indexPath,omitempty"`
}

func Collect(inputs []string, opts Options) ([]File, []string, error) {
	var files []File
	var warnings []string

	for _, input := range inputs {
		absInput, err := filepath.Abs(input)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve path %q: %w", input, err)
		}
		info, err := os.Stat(absInput)
		if err != nil {
			return nil, nil, fmt.Errorf("stat path %q: %w", absInput, err)
		}
		if info.IsDir() {
			discovered, dsWarnings, err := collectDir(absInput, opts)
			if err != nil {
				return nil, nil, err
			}
			files = append(files, discovered...)
			warnings = append(warnings, dsWarnings...)
			continue
		}

		file, warning, ok, err := classify(absInput, opts)
		if err != nil {
			return nil, nil, err
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if ok {
			files = append(files, file)
		}
	}

	files = dedupe(files)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, warnings, nil
}

func collectDir(root string, opts Options) ([]File, []string, error) {
	var files []File
	var warnings []string

	walkFn := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && !opts.Recursive {
				return filepath.SkipDir
			}
			return nil
		}
		file, warning, ok, err := classify(path, opts)
		if err != nil {
			return err
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if ok {
			files = append(files, file)
		}
		return nil
	}

	if opts.Recursive {
		if err := filepath.WalkDir(root, walkFn); err != nil {
			return nil, nil, fmt.Errorf("walk %q: %w", root, err)
		}
		return files, warnings, nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read directory %q: %w", root, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, warning, ok, err := classify(path, opts)
		if err != nil {
			return nil, nil, err
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if ok {
			files = append(files, file)
		}
	}
	return files, warnings, nil
}

func classify(path string, opts Options) (File, string, bool, error) {
	lower := strings.ToLower(path)

	switch {
	case isIndexFile(lower):
		return File{}, "", false, nil
	case strings.HasSuffix(lower, ".bam"):
		return indexedFile(path, "bam", "alignment", []string{path + ".bai", strings.TrimSuffix(path, ".bam") + ".bai"}, opts)
	case strings.HasSuffix(lower, ".cram"):
		return indexedFile(path, "cram", "alignment", []string{path + ".crai"}, opts)
	case strings.HasSuffix(lower, ".vcf.gz"):
		return indexedFile(path, "vcf", "variant", []string{path + ".tbi", path + ".csi"}, opts)
	case strings.HasSuffix(lower, ".bed.gz"):
		return indexedFile(path, "bed", "annotation", []string{path + ".tbi", path + ".csi"}, opts)
	case strings.HasSuffix(lower, ".bedgraph.gz"), strings.HasSuffix(lower, ".bg.gz"):
		return indexedFile(path, "bedgraph", "annotation", []string{path + ".tbi", path + ".csi"}, opts)
	case strings.HasSuffix(lower, ".bigwig"), strings.HasSuffix(lower, ".bw"):
		return plainFile(path, "bigwig", "wig"), "", true, nil
	case strings.HasSuffix(lower, ".bigbed"), strings.HasSuffix(lower, ".bb"):
		return plainFile(path, "bigbed", "annotation"), "", true, nil
	case strings.HasSuffix(lower, ".bed"):
		return plainFile(path, "bed", "annotation"), "plain BED included without an index: " + path, true, nil
	case strings.HasSuffix(lower, ".sam"):
		return plainFile(path, "sam", "alignment"), "plain SAM included without an index and may be slow or unsupported in IGV: " + path, true, nil
	default:
		return File{}, "", false, nil
	}
}

func indexedFile(path, format, trackType string, candidates []string, opts Options) (File, string, bool, error) {
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			file := plainFile(path, format, trackType)
			file.IndexPath = candidate
			return file, "", true, nil
		} else if !os.IsNotExist(err) {
			return File{}, "", false, fmt.Errorf("stat index %q: %w", candidate, err)
		}
	}

	if opts.AllowMissingIndex {
		return plainFile(path, format, trackType), "missing index allowed for " + path, true, nil
	}
	return File{}, "skipping indexed file without index: " + path, false, nil
}

func plainFile(path, format, trackType string) File {
	sum := sha1.Sum([]byte(path))
	return File{
		ID:        hex.EncodeToString(sum[:]),
		Name:      filepath.Base(path),
		Path:      path,
		Format:    format,
		TrackType: trackType,
	}
}

func isIndexFile(path string) bool {
	return strings.HasSuffix(path, ".bai") ||
		strings.HasSuffix(path, ".crai") ||
		strings.HasSuffix(path, ".tbi") ||
		strings.HasSuffix(path, ".csi")
}

func dedupe(files []File) []File {
	seen := make(map[string]struct{}, len(files))
	out := make([]File, 0, len(files))
	for _, file := range files {
		if _, ok := seen[file.Path]; ok {
			continue
		}
		seen[file.Path] = struct{}{}
		out = append(out, file)
	}
	return out
}
