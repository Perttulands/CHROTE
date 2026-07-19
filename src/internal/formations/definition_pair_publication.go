package formations

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

var errDefinitionPublicationUncertain = errors.New("definition_publication_uncertain")

type definitionPairContent struct {
	present bool
	raw     []byte
}

type definitionPairIdentity struct {
	present bool
	sha256  string
}

type definitionPairStateIdentity struct {
	board  definitionPairIdentity
	layout definitionPairIdentity
}

type definitionPairState struct {
	board  []byte
	layout definitionPairContent
}

type definitionPairPublicationRequest struct {
	expected  definitionPairStateIdentity
	candidate definitionPairState
	validate  func(definitionPairState, definitionPairState) error
	cas       func(definitionPairState) error
}

type definitionPairStage struct {
	definition *definitionFile
	name       string
	raw        []byte
	available  bool
}

type definitionPairStages struct {
	oldBoard  *definitionPairStage
	oldLayout *definitionPairStage
	newBoard  *definitionPairStage
	newLayout *definitionPairStage
}

func (s *Store) publishDefinitionPair(
	slug string,
	request definitionPairPublicationRequest,
	fault func(string) error,
) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	request.candidate = cloneDefinitionPairState(request.candidate)

	board, err := s.openBoardDefinition(slug, false)
	if err != nil {
		return err
	}
	defer board.close()
	if err := confirmPresentDefinitionPairFile(board); err != nil {
		return err
	}

	return board.withLock(func(board *definitionFile) error {
		if err := confirmPresentDefinitionPairFile(board); err != nil {
			return err
		}
		layoutDirectory, err := s.openDefinitionDirectoryWithLeafParentSync(
			layoutDefinitionKind,
			true,
			func() error {
				return runDefinitionPairFault(fault, "preflight:layout-directory-parent:dir-sync")
			},
		)
		if err != nil {
			return definitionPathError(err)
		}
		layout := &definitionFile{
			directory: layoutDirectory,
			name:      slug + layoutDefinitionKind.suffix,
			path: filepath.Join(
				s.workspaceRoot(),
				".formations",
				layoutDefinitionKind.directory,
				slug+layoutDefinitionKind.suffix,
			),
		}
		defer layout.close()

		return layout.withLock(func(layout *definitionFile) error {
			return publishDefinitionPairLocked(board, layout, request, fault)
		})
	})
}

func publishDefinitionPairLocked(
	board *definitionFile,
	layout *definitionFile,
	request definitionPairPublicationRequest,
	fault func(string) error,
) error {
	current, err := readDefinitionPairState(board, layout)
	if err != nil {
		return err
	}
	if request.validate != nil {
		if err := request.validate(
			cloneDefinitionPairState(current),
			cloneDefinitionPairState(request.candidate),
		); err != nil {
			return err
		}
	}

	if err := preflightDefinitionPair(board, layout, current, fault); err != nil {
		return err
	}
	if !equalDefinitionPairStateIdentity(
		definitionPairStateIdentityOf(current),
		request.expected,
	) {
		return ErrConflict
	}
	if request.cas != nil {
		if err := request.cas(cloneDefinitionPairState(current)); err != nil {
			return err
		}
	}

	stages, err := stageDefinitionPairRepresentations(board, layout, current, request.candidate, fault)
	if err != nil {
		return err
	}
	defer stages.cleanup()

	pinned, err := readDefinitionPairState(board, layout)
	if err != nil {
		return err
	}
	if !equalDefinitionPairStateIdentity(definitionPairStateIdentityOf(pinned), definitionPairStateIdentityOf(current)) {
		return ErrConflict
	}

	publicationErr, mutated := publishStagedDefinitionPair(board, layout, current, request.candidate, stages, fault)
	if publicationErr == nil || !mutated {
		return publicationErr
	}
	return reconcileDefinitionPair(
		board,
		layout,
		current,
		request.candidate,
		stages,
		publicationErr,
		fault,
	)
}

func preflightDefinitionPair(
	board *definitionFile,
	layout *definitionFile,
	current definitionPairState,
	fault func(string) error,
) error {
	if err := syncExactDefinitionPairFile(board, current.board, "preflight:board:file-sync", fault); err != nil {
		return err
	}
	if current.layout.present {
		if err := syncExactDefinitionPairFile(layout, current.layout.raw, "preflight:layout:file-sync", fault); err != nil {
			return err
		}
	} else if err := confirmAbsentDefinitionPairFile(layout, "", nil); err != nil {
		return err
	}
	if err := syncDefinitionPairDirectory(board.directory, "preflight:board:dir-sync", fault); err != nil {
		return err
	}
	return syncDefinitionPairDirectory(layout.directory, "preflight:layout:dir-sync", fault)
}

func stageDefinitionPairRepresentations(
	board *definitionFile,
	layout *definitionFile,
	oldState definitionPairState,
	newState definitionPairState,
	fault func(string) error,
) (*definitionPairStages, error) {
	stages := &definitionPairStages{}
	var err error
	if stages.oldBoard, err = stageDefinitionPairContent(board, oldState.board, "old-board", fault); err != nil {
		stages.cleanup()
		return nil, err
	}
	if oldState.layout.present {
		if stages.oldLayout, err = stageDefinitionPairContent(layout, oldState.layout.raw, "old-layout", fault); err != nil {
			stages.cleanup()
			return nil, err
		}
	}
	if stages.newBoard, err = stageDefinitionPairContent(board, newState.board, "new-board", fault); err != nil {
		stages.cleanup()
		return nil, err
	}
	if newState.layout.present {
		if stages.newLayout, err = stageDefinitionPairContent(layout, newState.layout.raw, "new-layout", fault); err != nil {
			stages.cleanup()
			return nil, err
		}
	}
	return stages, nil
}

func stageDefinitionPairContent(
	definition *definitionFile,
	raw []byte,
	role string,
	fault func(string) error,
) (_ *definitionPairStage, returnErr error) {
	if err := runDefinitionPairFault(fault, "stage:"+role+":create"); err != nil {
		return nil, err
	}
	stage := &definitionPairStage{
		definition: definition,
		name:       "." + definition.name + "." + newPrefixedID("pair"),
		raw:        append([]byte(nil), raw...),
		available:  true,
	}
	temporary, err := openDefinitionRegularFileAt(definition.directory, stage.name, syscall.O_WRONLY, true)
	if err != nil {
		return nil, definitionPathError(err)
	}
	defer func() {
		_ = temporary.Close()
		if returnErr != nil {
			_ = syscall.Unlinkat(int(definition.directory.Fd()), stage.name)
		}
	}()

	if err := runDefinitionPairFault(fault, "stage:"+role+":write"); err != nil {
		return nil, err
	}
	written, err := temporary.Write(stage.raw)
	if err != nil {
		return nil, err
	}
	if written != len(stage.raw) {
		return nil, io.ErrShortWrite
	}
	if err := runDefinitionPairFault(fault, "stage:"+role+":file-sync"); err != nil {
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	if err := runDefinitionPairFault(fault, "stage:"+role+":close"); err != nil {
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	return stage, nil
}

func (stages *definitionPairStages) cleanup() {
	if stages == nil {
		return
	}
	for _, stage := range []*definitionPairStage{
		stages.oldBoard,
		stages.oldLayout,
		stages.newBoard,
		stages.newLayout,
	} {
		if stage != nil && stage.available {
			_ = syscall.Unlinkat(int(stage.definition.directory.Fd()), stage.name)
			stage.available = false
		}
	}
}

func publishStagedDefinitionPair(
	board *definitionFile,
	layout *definitionFile,
	oldState definitionPairState,
	newState definitionPairState,
	stages *definitionPairStages,
	fault func(string) error,
) (error, bool) {
	mutated := false
	if newState.layout.present {
		if err := renameDefinitionPairStage(stages.newLayout, "publish:layout:rename", fault); err != nil {
			return err, mutated
		}
		mutated = true
		if err := syncExactDefinitionPairFile(layout, newState.layout.raw, "publish:layout:file-sync", fault); err != nil {
			return err, mutated
		}
		if err := syncDefinitionPairDirectory(layout.directory, "publish:layout:dir-sync", fault); err != nil {
			return err, mutated
		}
	} else {
		if oldState.layout.present {
			if err := runDefinitionPairFault(fault, "publish:layout:unlink"); err != nil {
				return err, mutated
			}
			if err := syscall.Unlinkat(int(layout.directory.Fd()), layout.name); err != nil {
				return definitionPathError(&os.PathError{Op: "unlinkat", Path: layout.name, Err: err}), mutated
			}
			mutated = true
		}
		if err := confirmAbsentDefinitionPairFile(layout, "publish:layout:absence-check", fault); err != nil {
			return err, mutated
		}
		if err := syncDefinitionPairDirectory(layout.directory, "publish:layout:dir-sync", fault); err != nil {
			return err, mutated
		}
	}

	if err := renameDefinitionPairStage(stages.newBoard, "publish:board:rename", fault); err != nil {
		return err, mutated
	}
	mutated = true
	if err := syncExactDefinitionPairFile(board, newState.board, "publish:board:file-sync", fault); err != nil {
		return err, mutated
	}
	if err := syncDefinitionPairDirectory(board.directory, "publish:board:dir-sync", fault); err != nil {
		return err, mutated
	}
	if err := runDefinitionPairFault(fault, "publish:board:dir-synced"); err != nil {
		return err, mutated
	}
	return nil, mutated
}

func reconcileDefinitionPair(
	board *definitionFile,
	layout *definitionFile,
	oldState definitionPairState,
	newState definitionPairState,
	stages *definitionPairStages,
	publicationErr error,
	fault func(string) error,
) error {
	state, err := readDefinitionPairState(board, layout)
	if err != nil {
		return errDefinitionPublicationUncertain
	}
	oldIdentity := definitionPairStateIdentityOf(oldState)
	newIdentity := definitionPairStateIdentityOf(newState)
	identity := definitionPairStateIdentityOf(state)

	switch {
	case equalDefinitionPairStateIdentity(identity, oldIdentity):
		if reconcileDefinitionPairToOld(board, layout, oldState, newState, stages, fault) == nil {
			return publicationErr
		}
		return errDefinitionPublicationUncertain
	case equalDefinitionPairStateIdentity(identity, newIdentity),
		equalDefinitionPairIdentity(identity.board, oldIdentity.board) &&
			equalDefinitionPairIdentity(identity.layout, newIdentity.layout):
		if reconcileDefinitionPairToNew(board, layout, oldState, newState, stages, fault) == nil {
			return nil
		}
		state, err = readDefinitionPairState(board, layout)
		if err != nil || !contractedDefinitionPairState(state, oldState, newState) {
			return errDefinitionPublicationUncertain
		}
		if reconcileDefinitionPairToOld(board, layout, oldState, newState, stages, fault) == nil {
			return publicationErr
		}
		return errDefinitionPublicationUncertain
	default:
		return errDefinitionPublicationUncertain
	}
}

func reconcileDefinitionPairToNew(
	board *definitionFile,
	layout *definitionFile,
	oldState definitionPairState,
	newState definitionPairState,
	stages *definitionPairStages,
	fault func(string) error,
) error {
	state, err := readDefinitionPairState(board, layout)
	if err != nil || !contractedDefinitionPairState(state, oldState, newState) {
		return errDefinitionPublicationUncertain
	}
	if newState.layout.present {
		if !equalDefinitionPairIdentity(
			definitionPairIdentityOfContent(state.layout),
			definitionPairIdentityOfContent(newState.layout),
		) {
			if err := renameDefinitionPairStage(stages.newLayout, "reconcile:new:layout:rename", fault); err != nil {
				return err
			}
		}
		if err := syncExactDefinitionPairFile(layout, newState.layout.raw, "reconcile:new:layout:file-sync", fault); err != nil {
			return err
		}
	} else {
		if state.layout.present {
			return errDefinitionPublicationUncertain
		}
		if err := confirmAbsentDefinitionPairFile(layout, "reconcile:new:layout:absence-check", fault); err != nil {
			return err
		}
	}
	if err := syncDefinitionPairDirectory(layout.directory, "reconcile:new:layout:dir-sync", fault); err != nil {
		return err
	}

	state, err = readDefinitionPairState(board, layout)
	if err != nil {
		return err
	}
	if !equalDefinitionPairIdentity(
		definitionPairIdentityOfBoard(state.board),
		definitionPairIdentityOfBoard(newState.board),
	) {
		if err := renameDefinitionPairStage(stages.newBoard, "reconcile:new:board:rename", fault); err != nil {
			return err
		}
	}
	if err := syncExactDefinitionPairFile(board, newState.board, "reconcile:new:board:file-sync", fault); err != nil {
		return err
	}
	if err := syncDefinitionPairDirectory(board.directory, "reconcile:new:board:dir-sync", fault); err != nil {
		return err
	}
	return runDefinitionPairFault(fault, "reconcile:new:board:dir-synced")
}

func reconcileDefinitionPairToOld(
	board *definitionFile,
	layout *definitionFile,
	oldState definitionPairState,
	newState definitionPairState,
	stages *definitionPairStages,
	fault func(string) error,
) error {
	state, err := readDefinitionPairState(board, layout)
	if err != nil || !contractedDefinitionPairState(state, oldState, newState) {
		return errDefinitionPublicationUncertain
	}
	if !equalDefinitionPairIdentity(
		definitionPairIdentityOfBoard(state.board),
		definitionPairIdentityOfBoard(oldState.board),
	) {
		if err := renameDefinitionPairStage(stages.oldBoard, "reconcile:old:board:rename", fault); err != nil {
			return err
		}
	}
	if err := syncExactDefinitionPairFile(board, oldState.board, "reconcile:old:board:file-sync", fault); err != nil {
		return err
	}
	if err := syncDefinitionPairDirectory(board.directory, "reconcile:old:board:dir-sync", fault); err != nil {
		return err
	}

	state, err = readDefinitionPairState(board, layout)
	if err != nil {
		return err
	}
	if oldState.layout.present {
		if !equalDefinitionPairIdentity(
			definitionPairIdentityOfContent(state.layout),
			definitionPairIdentityOfContent(oldState.layout),
		) {
			if err := renameDefinitionPairStage(stages.oldLayout, "reconcile:old:layout:rename", fault); err != nil {
				return err
			}
		}
		if err := syncExactDefinitionPairFile(layout, oldState.layout.raw, "reconcile:old:layout:file-sync", fault); err != nil {
			return err
		}
	} else {
		if state.layout.present {
			if err := runDefinitionPairFault(fault, "reconcile:old:layout:unlink"); err != nil {
				return err
			}
			if err := syscall.Unlinkat(int(layout.directory.Fd()), layout.name); err != nil {
				return definitionPathError(&os.PathError{Op: "unlinkat", Path: layout.name, Err: err})
			}
		}
		if err := confirmAbsentDefinitionPairFile(layout, "reconcile:old:layout:absence-check", fault); err != nil {
			return err
		}
	}
	if err := syncDefinitionPairDirectory(layout.directory, "reconcile:old:layout:dir-sync", fault); err != nil {
		return err
	}
	return runDefinitionPairFault(fault, "reconcile:old:layout:dir-synced")
}

func renameDefinitionPairStage(stage *definitionPairStage, step string, fault func(string) error) error {
	if stage == nil || !stage.available {
		return errors.New("paired definition stage is unavailable")
	}
	if err := verifyDefinitionPairStage(stage); err != nil {
		return err
	}
	if err := runDefinitionPairFault(fault, step); err != nil {
		return err
	}
	if err := syscall.Renameat(
		int(stage.definition.directory.Fd()),
		stage.name,
		int(stage.definition.directory.Fd()),
		stage.definition.name,
	); err != nil {
		return definitionPathError(&os.PathError{Op: "renameat", Path: stage.definition.name, Err: err})
	}
	stage.available = false
	return nil
}

func verifyDefinitionPairStage(stage *definitionPairStage) error {
	file, err := openDefinitionRegularFileAt(stage.definition.directory, stage.name, syscall.O_RDONLY, false)
	if err != nil {
		return definitionPathError(err)
	}
	raw, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !bytes.Equal(raw, stage.raw) {
		return ErrConflict
	}
	return nil
}

func readDefinitionPairState(board *definitionFile, layout *definitionFile) (definitionPairState, error) {
	boardContent, err := readDefinitionPairContent(board, false)
	if err != nil {
		return definitionPairState{}, err
	}
	layoutContent, err := readDefinitionPairContent(layout, true)
	if err != nil {
		return definitionPairState{}, err
	}
	return definitionPairState{
		board:  boardContent.raw,
		layout: layoutContent,
	}, nil
}

func readDefinitionPairContent(definition *definitionFile, optional bool) (definitionPairContent, error) {
	file, err := openDefinitionRegularFileAt(definition.directory, definition.name, syscall.O_RDONLY, false)
	if optional && errors.Is(err, os.ErrNotExist) {
		return definitionPairContent{}, nil
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return definitionPairContent{}, ErrNotFound
		}
		return definitionPairContent{}, definitionPathError(err)
	}
	raw, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return definitionPairContent{}, readErr
	}
	if closeErr != nil {
		return definitionPairContent{}, closeErr
	}
	return definitionPairContent{present: true, raw: raw}, nil
}

func syncExactDefinitionPairFile(
	definition *definitionFile,
	expected []byte,
	step string,
	fault func(string) error,
) error {
	file, err := openDefinitionRegularFileAt(definition.directory, definition.name, syscall.O_RDWR, false)
	if err != nil {
		return definitionPathError(err)
	}
	defer file.Close() //nolint:errcheck // the explicit durability boundary is fsync
	raw, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, expected) {
		return ErrConflict
	}
	if err := runDefinitionPairFault(fault, step); err != nil {
		return err
	}
	return file.Sync()
}

func confirmAbsentDefinitionPairFile(
	definition *definitionFile,
	step string,
	fault func(string) error,
) error {
	if step != "" {
		if err := runDefinitionPairFault(fault, step); err != nil {
			return err
		}
	}
	file, err := openDefinitionRegularFileAt(definition.directory, definition.name, syscall.O_RDONLY, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return definitionPathError(err)
	}
	_ = file.Close()
	return errors.New("paired definition expected an absent file")
}

func confirmPresentDefinitionPairFile(definition *definitionFile) error {
	file, err := openDefinitionRegularFileAt(definition.directory, definition.name, syscall.O_RDONLY, false)
	if err != nil {
		return definitionPathError(err)
	}
	return file.Close()
}

func syncDefinitionPairDirectory(directory *os.File, step string, fault func(string) error) error {
	if err := runDefinitionPairFault(fault, step); err != nil {
		return err
	}
	return directory.Sync()
}

func runDefinitionPairFault(fault func(string) error, step string) error {
	if fault == nil {
		return nil
	}
	return fault(step)
}

func contractedDefinitionPairState(current, oldState, newState definitionPairState) bool {
	currentIdentity := definitionPairStateIdentityOf(current)
	oldIdentity := definitionPairStateIdentityOf(oldState)
	newIdentity := definitionPairStateIdentityOf(newState)
	if equalDefinitionPairStateIdentity(currentIdentity, oldIdentity) ||
		equalDefinitionPairStateIdentity(currentIdentity, newIdentity) {
		return true
	}
	return equalDefinitionPairIdentity(currentIdentity.board, oldIdentity.board) &&
		equalDefinitionPairIdentity(currentIdentity.layout, newIdentity.layout)
}

func definitionPairStateIdentityOf(state definitionPairState) definitionPairStateIdentity {
	return definitionPairStateIdentity{
		board:  definitionPairIdentityOfBoard(state.board),
		layout: definitionPairIdentityOfContent(state.layout),
	}
}

func definitionPairIdentityOfBoard(raw []byte) definitionPairIdentity {
	return definitionPairIdentity{present: true, sha256: etag(raw)}
}

func definitionPairIdentityOfContent(content definitionPairContent) definitionPairIdentity {
	if !content.present {
		return definitionPairIdentity{}
	}
	return definitionPairIdentity{present: true, sha256: etag(content.raw)}
}

func equalDefinitionPairStateIdentity(left, right definitionPairStateIdentity) bool {
	return equalDefinitionPairIdentity(left.board, right.board) &&
		equalDefinitionPairIdentity(left.layout, right.layout)
}

func equalDefinitionPairIdentity(left, right definitionPairIdentity) bool {
	return left.present == right.present && left.sha256 == right.sha256
}

func cloneDefinitionPairState(state definitionPairState) definitionPairState {
	return definitionPairState{
		board: append([]byte(nil), state.board...),
		layout: definitionPairContent{
			present: state.layout.present,
			raw:     append([]byte(nil), state.layout.raw...),
		},
	}
}
