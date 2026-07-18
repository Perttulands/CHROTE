package formations

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefinitionReadsRejectExternalLinksWithoutMigration(t *testing.T) {
	definitions := []struct {
		name      string
		directory string
		filename  string
		private   string
		read      func(*Store) (string, error)
	}{
		{
			name:      "board",
			directory: "boards",
			filename:  "private.formation.toml",
			private: "id = \"brd_private\"\n" +
				"slug = \"private\"\n" +
				"title = \"Private authority record\"\n" +
				"rev = 1\n" +
				"privateAuthority = \"board-secret-must-not-escape\"\n",
			read: func(store *Store) (string, error) {
				document, err := store.ReadBoard("private")
				if err != nil {
					return "", err
				}
				return document.TOML, nil
			},
		},
		{
			name:      "layout",
			directory: "layout",
			filename:  "private.layout.toml",
			private: "boardId = \"brd_private\"\n" +
				"boardRev = 1\n" +
				"updatedAt = \"2026-07-18T12:00:00Z\"\n" +
				"privateAuthority = \"layout-secret-must-not-escape\"\n",
			read: func(store *Store) (string, error) {
				document, err := store.ReadLayout("private")
				if err != nil {
					return "", err
				}
				return document.TOML, nil
			},
		},
	}
	attacks := []string{"final_symlink", "parent_symlink", "hardlink"}

	for _, definition := range definitions {
		definition := definition
		for _, attack := range attacks {
			attack := attack
			t.Run(definition.name+"/"+attack, func(t *testing.T) {
				root := t.TempDir()
				workspace := filepath.Join(root, "workspace")
				privateDirectory := filepath.Join(root, "host-private", definition.directory)
				if err := os.MkdirAll(workspace, 0o755); err != nil {
					t.Fatalf("create workspace: %v", err)
				}
				if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
					t.Fatalf("create private directory: %v", err)
				}
				victimPath := filepath.Join(privateDirectory, definition.filename)
				if err := os.WriteFile(victimPath, []byte(definition.private), 0o600); err != nil {
					t.Fatalf("write private record: %v", err)
				}

				definitionDirectory := filepath.Join(workspace, ".formations", definition.directory)
				definitionPath := filepath.Join(definitionDirectory, definition.filename)
				switch attack {
				case "final_symlink":
					if err := os.MkdirAll(definitionDirectory, 0o755); err != nil {
						t.Fatalf("create definition directory: %v", err)
					}
					if err := os.Symlink(victimPath, definitionPath); err != nil {
						t.Fatalf("symlink private record: %v", err)
					}
				case "parent_symlink":
					if err := os.MkdirAll(filepath.Dir(definitionDirectory), 0o755); err != nil {
						t.Fatalf("create formations directory: %v", err)
					}
					if err := os.Symlink(privateDirectory, definitionDirectory); err != nil {
						t.Fatalf("symlink private definition directory: %v", err)
					}
				case "hardlink":
					if err := os.MkdirAll(definitionDirectory, 0o755); err != nil {
						t.Fatalf("create definition directory: %v", err)
					}
					if err := os.Link(victimPath, definitionPath); err != nil {
						t.Fatalf("hardlink private record: %v", err)
					}
				default:
					t.Fatalf("unknown attack %q", attack)
				}

				exposed, err := definition.read(NewStore(workspace))
				if err == nil {
					t.Errorf("read followed %s and exposed %q", attack, exposed)
				}
				victimAfter, readErr := os.ReadFile(victimPath)
				if readErr != nil {
					t.Fatalf("read private record after rejection: %v", readErr)
				}
				if string(victimAfter) != definition.private {
					t.Errorf("rejected read migrated or mutated private record:\n%s", victimAfter)
				}
				if _, statErr := os.Lstat(victimPath + ".lock"); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("rejected read created a lock beside the private record: %v", statErr)
				}

				switch attack {
				case "final_symlink":
					info, statErr := os.Lstat(definitionPath)
					if statErr != nil {
						t.Fatalf("lstat rejected final symlink: %v", statErr)
					}
					if info.Mode()&os.ModeSymlink == 0 {
						t.Errorf("rejected read replaced final symlink with mode %v", info.Mode())
					}
				case "parent_symlink":
					info, statErr := os.Lstat(definitionDirectory)
					if statErr != nil {
						t.Fatalf("lstat rejected parent symlink: %v", statErr)
					}
					if info.Mode()&os.ModeSymlink == 0 {
						t.Errorf("rejected read replaced parent symlink with mode %v", info.Mode())
					}
				case "hardlink":
					definitionInfo, statErr := os.Stat(definitionPath)
					if statErr != nil {
						t.Fatalf("stat rejected hardlink: %v", statErr)
					}
					victimInfo, statErr := os.Stat(victimPath)
					if statErr != nil {
						t.Fatalf("stat private record: %v", statErr)
					}
					if !os.SameFile(definitionInfo, victimInfo) {
						t.Error("rejected read replaced the hardlink binding")
					}
				}
			})
		}
	}
}

func TestDefinitionWritesRejectExternalLockLinksWithoutMutation(t *testing.T) {
	definitions := []struct {
		name    string
		path    func(*Store) string
		fixture string
		update  func(*Store) error
	}{
		{
			name:    "board",
			path:    func(store *Store) string { return store.BoardPath("private") },
			fixture: minimalBoard("private", 1),
			update: func(store *Store) error {
				board, err := store.ReadBoard("private")
				if err != nil {
					return err
				}
				title := "must not be written"
				_, err = store.UpdateBoardMetadata("private", BoardMetadataPatch{Title: &title}, WriteOptions{
					ExpectedETag: board.ETag,
					ExpectedRev:  board.Rev,
				})
				return err
			},
		},
		{
			name: "layout",
			path: func(store *Store) string { return store.LayoutPath("private") },
			fixture: "schema = 1\n" +
				"boardId = \"brd_private\"\n" +
				"boardRev = 1\n" +
				"updatedAt = \"2026-07-18T12:00:00Z\"\n",
			update: func(store *Store) error {
				layout, err := store.ReadLayout("private")
				if err != nil {
					return err
				}
				_, err = store.UpdateLayoutMetadata("private", LayoutMetadataPatch{
					UpdatedAt: time.Date(2026, 7, 18, 13, 0, 0, 0, time.UTC),
				}, WriteOptions{ExpectedETag: layout.ETag})
				return err
			},
		},
	}

	for _, definition := range definitions {
		definition := definition
		for _, attack := range []string{"symlink", "hardlink"} {
			attack := attack
			t.Run(definition.name+"/"+attack, func(t *testing.T) {
				root := t.TempDir()
				store := NewStore(filepath.Join(root, "workspace"))
				store.Now = fixedClock()
				definitionPath := definition.path(store)
				writeFixture(t, definitionPath, definition.fixture)
				definitionBefore := readFile(t, definitionPath)

				victimPath := filepath.Join(root, "host-private-lock")
				victimBefore := "private lock authority\n"
				if err := os.WriteFile(victimPath, []byte(victimBefore), 0o600); err != nil {
					t.Fatalf("write private lock record: %v", err)
				}
				lockPath := definitionPath + ".lock"
				switch attack {
				case "symlink":
					if err := os.Symlink(victimPath, lockPath); err != nil {
						t.Fatalf("symlink private lock record: %v", err)
					}
				case "hardlink":
					if err := os.Link(victimPath, lockPath); err != nil {
						t.Fatalf("hardlink private lock record: %v", err)
					}
				}

				if err := definition.update(store); err == nil {
					t.Errorf("write accepted %s lock substitution", attack)
				}
				if got := readFile(t, definitionPath); got != definitionBefore {
					t.Errorf("rejected lock substitution mutated definition:\n%s", got)
				}
				if got := readFile(t, victimPath); got != victimBefore {
					t.Errorf("rejected lock substitution mutated private bytes: %q", got)
				}
				info, err := os.Stat(victimPath)
				if err != nil {
					t.Fatalf("stat private lock record: %v", err)
				}
				if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
					t.Errorf("rejected lock substitution changed private mode to %04o, want %04o", got, want)
				}
				if attack == "symlink" {
					info, err := os.Lstat(lockPath)
					if err != nil {
						t.Fatalf("lstat rejected lock symlink: %v", err)
					}
					if info.Mode()&os.ModeSymlink == 0 {
						t.Errorf("rejected write replaced lock symlink with mode %v", info.Mode())
					}
				} else {
					lockInfo, err := os.Stat(lockPath)
					if err != nil {
						t.Fatalf("stat rejected lock hardlink: %v", err)
					}
					if !os.SameFile(lockInfo, info) {
						t.Error("rejected write replaced lock hardlink")
					}
				}
			})
		}
	}
}

func TestDefinitionWriterRepairsValidatedSingleLinkLockMode(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("private"), minimalBoard("private", 1))
	board, err := store.ReadBoard("private")
	if err != nil {
		t.Fatalf("read board precondition: %v", err)
	}
	lockPath := store.BoardPath("private") + ".lock"
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatalf("write legitimate single-link lock: %v", err)
	}
	title := "Updated"
	if _, err := store.UpdateBoardMetadata("private", BoardMetadataPatch{Title: &title}, WriteOptions{
		ExpectedETag: board.ETag,
		ExpectedRev:  board.Rev,
	}); err != nil {
		t.Fatalf("update with legitimate single-link lock: %v", err)
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat repaired lock: %v", err)
	}
	if got := info.Mode().Perm(); got != sharedFileMode {
		t.Fatalf("repaired lock mode = %04o, want %04o", got, sharedFileMode)
	}
}

func TestLegacyMigrationRepairsValidatedDefinitionDirectoryMode(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("legacy"), "id = \"brd_legacy\"\nslug = \"legacy\"\ntitle = \"Legacy\"\nrev = 1\n")
	directory := filepath.Dir(store.BoardPath("legacy"))
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatalf("set legacy directory mode: %v", err)
	}

	if _, err := store.ReadBoard("legacy"); err != nil {
		t.Fatalf("migrate legacy board: %v", err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("stat migrated definition directory: %v", err)
	}
	if !hasSharedDirMode(info.Mode()) {
		t.Fatalf("migrated definition directory mode = %v, want shared setgid 0770", info.Mode())
	}
}

func TestListBoardsRejectsLinkedDefinition(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "workspace"))
	privateBoard := filepath.Join(root, "host-private-board.toml")
	if err := os.WriteFile(privateBoard, []byte(minimalBoard("private", 1)), 0o600); err != nil {
		t.Fatalf("write private board: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(store.BoardPath("private")), 0o755); err != nil {
		t.Fatalf("create board directory: %v", err)
	}
	if err := os.Symlink(privateBoard, store.BoardPath("private")); err != nil {
		t.Fatalf("symlink private board: %v", err)
	}

	if _, err := store.ListBoards(); err == nil {
		t.Fatal("list boards accepted linked definition")
	}
}

func TestConfiguredWorkspaceSymlinkStillSupportsDefinitionPersistence(t *testing.T) {
	root := t.TempDir()
	actualWorkspace := filepath.Join(root, "actual-workspace")
	configuredWorkspace := filepath.Join(root, "configured-workspace")
	if err := os.MkdirAll(actualWorkspace, 0o755); err != nil {
		t.Fatalf("create actual workspace: %v", err)
	}
	if err := os.Symlink(actualWorkspace, configuredWorkspace); err != nil {
		t.Fatalf("symlink configured workspace: %v", err)
	}
	store := NewStore(configuredWorkspace)
	store.Now = fixedClock()

	created, err := store.CreateBoard(BoardCreateRequest{Slug: "linked", Title: "Linked workspace"})
	if err != nil {
		t.Fatalf("create board through configured workspace symlink: %v", err)
	}
	if created.Slug != "linked" || !strings.Contains(readFile(t, store.BoardPath("linked")), `title = "Linked workspace"`) {
		t.Fatalf("created board = %#v, want board persisted through configured workspace symlink", created)
	}
}

func TestFormationAuthoringRejectsExternalBoardLock(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "workspace"))
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("private"), minimalBoard("private", 1))
	board, err := store.ReadBoard("private")
	if err != nil {
		t.Fatalf("read board precondition: %v", err)
	}
	boardBefore := readFile(t, store.BoardPath("private"))
	victimPath := filepath.Join(root, "host-private-authoring-lock")
	if err := os.WriteFile(victimPath, []byte("private authority\n"), 0o600); err != nil {
		t.Fatalf("write private lock: %v", err)
	}
	if err := os.Symlink(victimPath, store.BoardPath("private")+".lock"); err != nil {
		t.Fatalf("symlink private lock: %v", err)
	}

	_, err = store.CreateFormation("private", FormationCreateRequest{Type: FormationTypeSolo, Title: "Must not exist"}, WriteOptions{
		ExpectedETag: board.ETag,
		ExpectedRev:  board.Rev,
	})
	if err == nil {
		t.Error("formation authoring accepted external board lock")
	}
	if got := readFile(t, store.BoardPath("private")); got != boardBefore {
		t.Errorf("rejected formation authoring mutated board:\n%s", got)
	}
	assertPrivateDefinitionLockUnchanged(t, victimPath)
}

func TestLayoutAuthoringRejectsExternalLayoutLock(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "workspace"))
	store.Now = fixedClock()
	writeFixture(t, store.LayoutPath("private"), "schema = 1\nboardId = \"brd_private\"\nboardRev = 1\nupdatedAt = \"2026-07-18T12:00:00Z\"\n")
	layout, err := store.ReadLayout("private")
	if err != nil {
		t.Fatalf("read layout precondition: %v", err)
	}
	layoutBefore := readFile(t, store.LayoutPath("private"))
	victimPath := filepath.Join(root, "host-private-layout-authoring-lock")
	if err := os.WriteFile(victimPath, []byte("private authority\n"), 0o600); err != nil {
		t.Fatalf("write private lock: %v", err)
	}
	if err := os.Link(victimPath, store.LayoutPath("private")+".lock"); err != nil {
		t.Fatalf("hardlink private lock: %v", err)
	}

	_, err = store.UpdateLayoutNodes("private", []LayoutNode{{ID: "fmn_private", X: 10, Y: 20}}, WriteOptions{ExpectedETag: layout.ETag})
	if err == nil {
		t.Error("layout authoring accepted external layout lock")
	}
	if got := readFile(t, store.LayoutPath("private")); got != layoutBefore {
		t.Errorf("rejected layout authoring mutated layout:\n%s", got)
	}
	assertPrivateDefinitionLockUnchanged(t, victimPath)
}

func TestArrangeLayoutRejectsExternalBoardLock(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "workspace"))
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("private"), minimalBoard("private", 1))
	victimPath := filepath.Join(root, "host-private-arrange-lock")
	if err := os.WriteFile(victimPath, []byte("private authority\n"), 0o600); err != nil {
		t.Fatalf("write private lock: %v", err)
	}
	if err := os.Symlink(victimPath, store.BoardPath("private")+".lock"); err != nil {
		t.Fatalf("symlink private lock: %v", err)
	}

	if _, err := store.ArrangeLayout("private", WriteOptions{ExpectedETag: "*"}); err == nil {
		t.Error("arrange accepted external board lock")
	}
	assertPrivateDefinitionLockUnchanged(t, victimPath)
	if _, err := os.Stat(store.LayoutPath("private")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rejected arrange created layout: %v", err)
	}
}

func TestStartRunRejectsExternalBoardLinkBeforeSnapshot(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	if _, err := personas.CreatePersona(CreatePersonaRequest{
		ID:           "scout",
		DisplayName:  "Scout",
		Kind:         "specialist",
		Capabilities: []string{"research"},
		Harness:      "openai-codex",
	}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	privateBoard := filepath.Join(filepath.Dir(store.Workspace), "host-private-board.toml")
	privateRaw := s4RunBoardFixture()
	if err := os.WriteFile(privateBoard, []byte(privateRaw), 0o600); err != nil {
		t.Fatalf("write private board: %v", err)
	}
	boardPath := store.BoardPath("session-search")
	if err := os.MkdirAll(filepath.Dir(boardPath), 0o755); err != nil {
		t.Fatalf("create board directory: %v", err)
	}
	if err := os.Symlink(privateBoard, boardPath); err != nil {
		t.Fatalf("symlink private board: %v", err)
	}

	_, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: etag([]byte(privateRaw)),
		ExpectedBoardRev:  7,
		Personas:          personas,
	})
	if err == nil {
		t.Error("start run accepted external board link")
	}
	if got := readFile(t, privateBoard); got != privateRaw {
		t.Error("rejected run start mutated private board")
	}
	info, statErr := os.Lstat(boardPath)
	if statErr != nil {
		t.Fatalf("lstat rejected board link: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("rejected run start replaced board link with mode %v", info.Mode())
	}
	runEntries, readErr := os.ReadDir(filepath.Join(store.Workspace, ".formations", "runs", "session-search"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("read run artifacts after rejection: %v", readErr)
	}
	if len(runEntries) != 0 {
		t.Errorf("rejected run start materialized %d artifacts from private board", len(runEntries))
	}
}

func assertPrivateDefinitionLockUnchanged(t *testing.T, victimPath string) {
	t.Helper()
	if got := readFile(t, victimPath); got != "private authority\n" {
		t.Errorf("rejected definition write mutated private lock bytes: %q", got)
	}
	info, err := os.Stat(victimPath)
	if err != nil {
		t.Fatalf("stat private lock: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("rejected definition write changed private lock mode to %04o, want %04o", got, want)
	}
}
