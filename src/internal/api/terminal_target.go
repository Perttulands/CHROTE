package api

// TerminalTarget is a resolved terminal destination: the tmux socket to attach
// on, the directory the attach starts in, and the Unix user both belong to.
type TerminalTarget struct {
	Socket   string
	WorkDir  string
	UnixUser string
}

// ResolveTerminalTarget is CHROTE's single implementation of tmux socket and
// working-directory resolution, exported so the terminal transport uses the
// very rules that session listing uses. The transport once duplicated them in
// shell, and the two copies drifted; there is no second copy now.
func ResolveTerminalTarget(unixUser string) (TerminalTarget, error) {
	target, err := NewTmuxHandler().targetForUnixUser(unixUser)
	if err != nil {
		return TerminalTarget{}, err
	}
	return TerminalTarget{
		Socket:   target.socket,
		WorkDir:  target.workDir,
		UnixUser: target.unixUser,
	}, nil
}
