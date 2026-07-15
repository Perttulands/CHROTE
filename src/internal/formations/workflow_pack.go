package formations

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const workflowPackRootToken = "{{packRoot}}"

type WorkflowPack struct {
	Schema         int    `json:"schema"`
	ID             string `json:"id"`
	Version        string `json:"version"`
	Title          string `json:"title"`
	BoardTemplate  string `json:"board"`
	LayoutTemplate string `json:"layout,omitempty"`
	MissionID      string `json:"missionId"`
	License        string `json:"license"`
	LicenseFile    string `json:"licenseFile"`
	Digest         string `json:"digest"`
	Root           string `json:"-"`
}

type WorkflowInstantiateRequest struct {
	Slug      string
	Title     string
	Goal      string
	UpdatedBy string
}

type WorkflowInstantiation struct {
	Pack          WorkflowPack    `json:"pack"`
	Board         *BoardDocument  `json:"board"`
	Layout        *LayoutDocument `json:"layout,omitempty"`
	InstalledRoot string          `json:"installedRoot"`
}

func LoadWorkflowPack(root string) (*WorkflowPack, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("%w: workflow pack path is required", ErrNotFound)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workflow pack root: %w", err)
	}
	absoluteRoot, err = filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workflow pack root: %w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: workflow pack root is not a directory", ErrNotFound)
	}
	manifestPath := filepath.Join(absoluteRoot, "pack.toml")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: pack.toml", ErrNotFound)
		}
		return nil, err
	}
	doc := parseTOMLDocument(manifestRaw)
	pack := &WorkflowPack{
		Schema:         doc.intValue("schema"),
		ID:             doc.stringValue("id"),
		Version:        doc.stringValue("version"),
		Title:          doc.stringValue("title"),
		BoardTemplate:  doc.stringValue("board"),
		LayoutTemplate: doc.stringValue("layout"),
		MissionID:      doc.stringValue("missionId"),
		License:        doc.stringValue("license"),
		LicenseFile:    doc.stringValue("licenseFile"),
		Root:           absoluteRoot,
	}
	if pack.Schema != 1 {
		return nil, fmt.Errorf("%w: workflow pack schema %d", ErrUnsupportedSchema, pack.Schema)
	}
	if err := validateSlug(pack.ID); err != nil {
		return nil, fmt.Errorf("%w: invalid workflow pack id %q", err, pack.ID)
	}
	if !validPackSegment(pack.Version) {
		return nil, fmt.Errorf("%w: invalid workflow pack version %q", ErrInvalidSlug, pack.Version)
	}
	if strings.TrimSpace(pack.Title) == "" || strings.TrimSpace(pack.MissionID) == "" || strings.TrimSpace(pack.License) == "" {
		return nil, fmt.Errorf("%w: workflow pack title, missionId, and license are required", ErrInvalidSlug)
	}
	if pack.LicenseFile == "" {
		pack.LicenseFile = "LICENSE"
	}
	boardPath, err := resolveWorkflowPackFile(absoluteRoot, pack.BoardTemplate)
	if err != nil {
		return nil, err
	}
	boardRaw, err := os.ReadFile(boardPath)
	if err != nil {
		return nil, err
	}
	board, err := parseBoard(boardRaw)
	if err != nil {
		return nil, fmt.Errorf("parse workflow board template: %w", err)
	}
	if !boardHasMission(board, pack.MissionID) {
		return nil, fmt.Errorf("%w: workflow mission %q is absent from board template", ErrNotFound, pack.MissionID)
	}
	if report := ValidateBoard(board); len(report.Errors) > 0 {
		return nil, fmt.Errorf("workflow board template invalid: %s", summarizeBoardErrors(report.Errors))
	}
	if pack.LayoutTemplate != "" {
		layoutPath, err := resolveWorkflowPackFile(absoluteRoot, pack.LayoutTemplate)
		if err != nil {
			return nil, err
		}
		layoutRaw, err := os.ReadFile(layoutPath)
		if err != nil {
			return nil, err
		}
		layout, err := parseLayout(layoutRaw)
		if err != nil {
			return nil, fmt.Errorf("parse workflow layout template: %w", err)
		}
		if layout.BoardID != board.ID {
			return nil, fmt.Errorf("%w: layout boardId %q does not match board id %q", ErrConflict, layout.BoardID, board.ID)
		}
		if err := validateTemplateLayout(board, layout); err != nil {
			return nil, err
		}
	}
	if _, err := resolveWorkflowPackFile(absoluteRoot, pack.LicenseFile); err != nil {
		return nil, fmt.Errorf("workflow pack license file: %w", err)
	}
	pack.Digest, err = workflowPackDigest(absoluteRoot)
	if err != nil {
		return nil, err
	}
	return pack, nil
}

func (s *Store) InstantiateWorkflowPack(root string, req WorkflowInstantiateRequest) (*WorkflowInstantiation, error) {
	if err := validateSlug(req.Slug); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Goal) == "" {
		return nil, fmt.Errorf("%w: workflow title and goal are required", ErrInvalidSlug)
	}
	pack, err := LoadWorkflowPack(root)
	if err != nil {
		return nil, err
	}
	workspace, err := canonicalWorkflowWorkspace(s.Workspace)
	if err != nil {
		return nil, err
	}
	packParent, err := ensureWorkflowWorkspaceDir(workspace, ".formations", "packs", pack.ID)
	if err != nil {
		return nil, err
	}
	boardDir, err := ensureWorkflowWorkspaceDir(workspace, ".formations", "boards")
	if err != nil {
		return nil, err
	}
	layoutDir, err := ensureWorkflowWorkspaceDir(workspace, ".formations", "layout")
	if err != nil {
		return nil, err
	}
	installedRoot := filepath.Join(".formations", "packs", pack.ID, pack.Version)
	installedPath := filepath.Join(packParent, pack.Version)
	if err := installWorkflowPack(pack, installedPath); err != nil {
		return nil, err
	}
	if err := requireWorkflowPackDigest(installedPath, pack.Digest); err != nil {
		return nil, err
	}

	boardTemplatePath, err := resolveWorkflowPackFile(installedPath, pack.BoardTemplate)
	if err != nil {
		return nil, err
	}
	boardRaw, err := os.ReadFile(boardTemplatePath)
	if err != nil {
		return nil, err
	}
	boardRaw = []byte(strings.ReplaceAll(string(boardRaw), workflowPackRootToken, filepath.ToSlash(installedRoot)))
	templateBoard, err := parseBoard(boardRaw)
	if err != nil {
		return nil, err
	}
	idMap, err := freshWorkflowIDs(templateBoard)
	if err != nil {
		return nil, err
	}
	boardRaw = replaceWorkflowIDs(boardRaw, idMap)
	missionID := idMap[pack.MissionID]
	doc := parseTOMLDocument(boardRaw)
	doc.setScalar("schema", renderInt(CurrentSchema))
	doc.setScalar("slug", renderString(req.Slug))
	doc.setScalar("title", renderString(strings.TrimSpace(req.Title)))
	doc.setScalar("rev", "1")
	doc.setScalar("updatedBy", renderString(strings.TrimSpace(req.UpdatedBy)))
	doc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))
	doc.setScalar("workflowPackId", renderString(pack.ID))
	doc.setScalar("workflowPackVersion", renderString(pack.Version))
	doc.setScalar("workflowPackDigest", renderString(pack.Digest))
	boardRaw = doc.bytes()
	lines := splitLines(boardRaw)
	missionStart, missionEnd, ok := findMissionBlockByID(lines, missionID)
	if !ok {
		return nil, fmt.Errorf("%w: remapped workflow mission %q", ErrNotFound, missionID)
	}
	lines = setScalarInLineRange(lines, missionStart+1, missionEnd, "goal", renderString(strings.TrimSpace(req.Goal)))
	boardRaw = renderTOMLLines(lines)
	board, err := parseBoard(boardRaw)
	if err != nil {
		return nil, err
	}
	if report := ValidateBoard(board); len(report.Errors) > 0 {
		return nil, fmt.Errorf("instantiated workflow board invalid: %s", summarizeBoardErrors(report.Errors))
	}

	var layoutRaw []byte
	if pack.LayoutTemplate != "" {
		layoutTemplatePath, err := resolveWorkflowPackFile(installedPath, pack.LayoutTemplate)
		if err != nil {
			return nil, err
		}
		layoutRaw, err = os.ReadFile(layoutTemplatePath)
		if err != nil {
			return nil, err
		}
		layoutRaw = replaceWorkflowIDs(layoutRaw, idMap)
		layoutDoc := parseTOMLDocument(layoutRaw)
		layoutDoc.setScalar("schema", renderInt(CurrentSchema))
		layoutDoc.setScalar("boardId", renderString(board.ID))
		layoutDoc.setScalar("boardRev", "1")
		layoutDoc.setScalar("updatedAt", renderString(s.now().Format(time.RFC3339)))
		layoutRaw = layoutDoc.bytes()
		layout, err := parseLayout(layoutRaw)
		if err != nil {
			return nil, err
		}
		if err := validateTemplateLayout(board, layout); err != nil {
			return nil, err
		}
	}
	if err := requireWorkflowPackDigest(installedPath, pack.Digest); err != nil {
		return nil, err
	}

	boardPath := filepath.Join(boardDir, req.Slug+".formation.toml")
	var result *WorkflowInstantiation
	err = withFileLock(boardPath, func() error {
		if _, err := os.Stat(boardPath); err == nil {
			return ErrAlreadyExists
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		layoutPath := filepath.Join(layoutDir, req.Slug+".layout.toml")
		if len(layoutRaw) > 0 {
			if _, err := os.Stat(layoutPath); err == nil {
				return ErrAlreadyExists
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		var layout *LayoutDocument
		layoutPublished := false
		if len(layoutRaw) > 0 {
			layoutPublished, err = writeAtomicNoReplace(layoutPath, layoutRaw)
			if err != nil {
				if layoutPublished {
					removeErr := os.Remove(layoutPath)
					syncErr := syncDirectory(layoutDir)
					return errors.Join(err, removeErr, syncErr)
				}
				return err
			}
			layout, err = parseLayout(layoutRaw)
			if err != nil {
				removeErr := os.Remove(layoutPath)
				syncErr := syncDirectory(layoutDir)
				return errors.Join(err, removeErr, syncErr)
			}
		}
		// The board is the publication marker. Layout is durable first, so
		// readers never observe an instance whose declared layout is missing.
		boardPublished, err := writeAtomicNoReplace(boardPath, boardRaw)
		if err != nil {
			if !boardPublished && layoutPublished {
				removeErr := os.Remove(layoutPath)
				syncErr := syncDirectory(layoutDir)
				return errors.Join(err, removeErr, syncErr)
			}
			return err
		}
		created, err := parseBoard(boardRaw)
		if err != nil {
			return err
		}
		publicPack := *pack
		publicPack.Root = ""
		result = &WorkflowInstantiation{
			Pack:          publicPack,
			Board:         created,
			Layout:        layout,
			InstalledRoot: filepath.ToSlash(installedRoot),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validPackSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return !strings.Contains(value, "..")
}

func resolveWorkflowPackFile(root, relative string) (string, error) {
	relative = strings.TrimSpace(relative)
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if relative == "" || filepath.IsAbs(relative) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: workflow pack file %q is outside pack", ErrInvalidSlug, relative)
	}
	candidate := filepath.Join(root, cleaned)
	candidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	inside, err := pathInside(candidate, root)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("%w: workflow pack file %q is outside pack", ErrInvalidSlug, relative)
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: workflow pack file %q is not regular", ErrInvalidSlug, relative)
	}
	return candidate, nil
}

func canonicalWorkflowWorkspace(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "."
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("workflow workspace is unavailable: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: workflow workspace is not a directory", ErrConflict)
	}
	return resolved, nil
}

func ensureWorkflowWorkspaceDir(workspace string, components ...string) (string, error) {
	current := workspace
	for _, component := range components {
		if !validPackSegment(component) && component != ".formations" {
			return "", fmt.Errorf("%w: unsafe workflow workspace path segment %q", ErrInvalidSlug, component)
		}
		next := filepath.Join(current, component)
		info, err := os.Lstat(next)
		if os.IsNotExist(err) {
			if err := os.Mkdir(next, sharedDirMode); err != nil && !os.IsExist(err) {
				return "", err
			}
			info, err = os.Lstat(next)
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: workflow workspace path %q is a symlink", ErrConflict, next)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%w: workflow workspace path %q is not a directory", ErrConflict, next)
		}
		current = next
	}
	return current, nil
}

func requireWorkflowPackDigest(root, expected string) error {
	actual, err := workflowPackDigest(root)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%w: installed workflow pack changed during instantiation", ErrConflict)
	}
	return nil
}

func installWorkflowPack(pack *WorkflowPack, destination string) error {
	if info, err := os.Lstat(destination); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: workflow pack destination is a symlink", ErrConflict)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: workflow pack destination is not a directory", ErrConflict)
		}
		digest, err := workflowPackDigest(destination)
		if err != nil {
			return err
		}
		if digest != pack.Digest {
			return fmt.Errorf("%w: installed workflow pack %s@%s differs from source", ErrConflict, pack.ID, pack.Version)
		}
		return syncDirectory(filepath.Dir(destination))
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".pack-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := copyWorkflowPackTree(pack.Root, stage); err != nil {
		return err
	}
	stagedDigest, err := workflowPackDigest(stage)
	if err != nil {
		return err
	}
	if stagedDigest != pack.Digest {
		return fmt.Errorf("%w: workflow pack source changed while staging %s@%s", ErrConflict, pack.ID, pack.Version)
	}
	if err := syncWorkflowPackTree(stage); err != nil {
		return err
	}
	if err := publishDirectoryNoReplace(stage, destination); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: workflow pack destination appeared concurrently", ErrConflict)
		}
		return err
	}
	return syncDirectory(parent)
}

func syncWorkflowPackTree(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%w: workflow pack staging encountered non-regular file %q", ErrConflict, path)
		}
		handle, err := os.Open(path)
		if err != nil {
			return err
		}
		return errors.Join(handle.Sync(), handle.Close())
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool {
		return len(directories[i]) > len(directories[j])
	})
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func copyWorkflowPackTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: workflow packs may not contain symlinks (%s)", ErrInvalidSlug, rel)
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: workflow pack entry %s is not a regular file", ErrInvalidSlug, rel)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if mode&0o111 == 0 {
			mode = 0o644
		} else {
			mode = 0o755
		}
		return os.WriteFile(target, raw, mode)
	})
}

func workflowPackDigest(root string) (string, error) {
	type fileHash struct {
		path string
		sum  string
	}
	var files []fileHash
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: workflow packs may not contain symlinks", ErrInvalidSlug)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: workflow pack entry is not regular", ErrInvalidSlug)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		files = append(files, fileHash{path: filepath.ToSlash(rel), sum: hex.EncodeToString(sum[:])})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	h := sha256.New()
	for _, file := range files {
		_, _ = h.Write([]byte(file.path))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(file.sum))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func freshWorkflowIDs(board *BoardDocument) (map[string]string, error) {
	mapping := map[string]string{}
	add := func(old, prefix string) error {
		if strings.TrimSpace(old) == "" {
			return fmt.Errorf("%w: workflow template contains an empty id", ErrInvalidSlug)
		}
		if _, exists := mapping[old]; exists {
			return fmt.Errorf("%w: workflow template id %q is duplicated", ErrConflict, old)
		}
		mapping[old] = newPrefixedID(prefix)
		return nil
	}
	if err := add(board.ID, "brd"); err != nil {
		return nil, err
	}
	for _, mission := range board.Missions {
		if err := add(mission.ID, "mis"); err != nil {
			return nil, err
		}
	}
	for _, formation := range board.Formations {
		if err := add(formation.ID, "fmn"); err != nil {
			return nil, err
		}
		for _, port := range append(append([]FormationPort{}, formation.Inputs...), formation.Outputs...) {
			if err := add(port.ID, "port"); err != nil {
				return nil, err
			}
		}
		for _, slot := range formation.Slots {
			if err := add(slot.ID, "slot"); err != nil {
				return nil, err
			}
		}
		if formation.Verification != nil {
			if err := add(formation.Verification.ID, "ver"); err != nil {
				return nil, err
			}
		}
	}
	for _, gate := range board.Gates {
		if err := add(gate.ID, "gate"); err != nil {
			return nil, err
		}
	}
	for _, connection := range board.Connections {
		if err := add(connection.ID, "edge"); err != nil {
			return nil, err
		}
	}
	return mapping, nil
}

func replaceWorkflowIDs(raw []byte, mapping map[string]string) []byte {
	lines := splitLines(raw)
	for index, line := range lines {
		key, value, ok := tomlKeyValue(line.body)
		if !ok {
			continue
		}
		switch key {
		case "id", "boardId":
			if replacement, exists := mapping[value]; exists {
				lines[index].body = replaceWorkflowTOMLValue(line.body, renderString(replacement))
			}
		case "from", "to":
			nodeID, portID := endpointParts(value)
			replacementNode, nodeChanged := mapping[nodeID]
			if !nodeChanged {
				replacementNode = nodeID
			}
			replacementPort, portChanged := mapping[portID]
			if !portChanged {
				replacementPort = portID
			}
			if nodeChanged || portChanged {
				lines[index].body = replaceWorkflowTOMLValue(line.body, renderString(replacementNode+":"+replacementPort))
			}
		}
	}
	return renderTOMLLines(lines)
}

func replaceWorkflowTOMLValue(line, value string) string {
	equals := strings.Index(line, "=")
	if equals < 0 {
		return line
	}
	return strings.TrimRight(line[:equals], " 	") + " = " + value
}

func validateTemplateLayout(board *BoardDocument, layout *LayoutDocument) error {
	knownNodes := map[string]bool{}
	for _, mission := range board.Missions {
		knownNodes[mission.ID] = true
	}
	for _, formation := range board.Formations {
		knownNodes[formation.ID] = true
	}
	for _, gate := range board.Gates {
		knownNodes[gate.ID] = true
	}
	knownEdges := map[string]bool{}
	for _, connection := range board.Connections {
		knownEdges[connection.ID] = true
	}
	for _, node := range layout.Nodes {
		if !knownNodes[node.ID] {
			return fmt.Errorf("%w: layout node %q is absent from board", ErrNotFound, node.ID)
		}
	}
	for _, edge := range layout.Edges {
		if !knownEdges[edge.ID] {
			return fmt.Errorf("%w: layout edge %q is absent from board", ErrNotFound, edge.ID)
		}
	}
	return nil
}

func boardHasMission(board *BoardDocument, missionID string) bool {
	for _, mission := range board.Missions {
		if mission.ID == missionID {
			return true
		}
	}
	return false
}

func summarizeBoardErrors(findings []BoardFinding) string {
	parts := make([]string, 0, len(findings))
	for _, finding := range findings {
		parts = append(parts, finding.Code+":"+finding.NodeID+":"+finding.Message)
	}
	return strings.Join(parts, "; ")
}
