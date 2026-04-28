package analyze

import (
	"os"
	"path/filepath"
	"strings"

	"roodox_server/internal/fs"
)

type BuildUnit struct {
	Path string
	Type string // cmake / make / vsproj / xcodeproj
}

type Analyzer struct {
	root string
}

func NewAnalyzer(root string) *Analyzer {
	return &Analyzer{root: root}
}

func detectBuildUnit(absPath string, entry os.DirEntry) (string, bool) {
	name := entry.Name()

	if name == "CMakeLists.txt" {
		return "cmake", true
	}
	if name == "Makefile" {
		return "make", true
	}
	if strings.HasSuffix(name, ".sln") || strings.HasSuffix(name, ".vcxproj") {
		return "vsproj", true
	}
	if strings.HasSuffix(name, ".xcodeproj") {
		return "xcodeproj", true
	}

	return "", false
}

func (a *Analyzer) Scan(relRoot string) ([]BuildUnit, error) {
	rootAbs, err := filepath.Abs(a.root)
	if err != nil {
		return nil, err
	}
	rootAbs = filepath.Clean(rootAbs)

	fullRoot, err := fs.ResolvePath(rootAbs, relRoot)
	if err != nil {
		return nil, err
	}

	units := []BuildUnit{}
	err = filepath.WalkDir(fullRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != fullRoot && fs.ShouldIgnoreInProjectScan(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		typ, ok := detectBuildUnit(path, d)
		if !ok {
			return nil
		}

		rel, err := filepath.Rel(rootAbs, filepath.Dir(path))
		if err != nil {
			return err
		}
		rel, err = fs.NormalizeRelativePath(rel)
		if err != nil {
			return err
		}

		units = append(units, BuildUnit{
			Path: rel,
			Type: typ,
		})
		return nil
	})
	return units, err
}
