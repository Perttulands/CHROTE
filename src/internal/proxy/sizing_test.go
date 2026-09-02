package proxy

import (
	"reflect"
	"strings"
	"testing"
)

// The sizing seat is decided by reading tmux back, so the reading is what has
// to be right: a client_flags list mixes settable flags with status ones, and
// one wrong match either hands a window to two clients or to none.
func TestSizingClients_ReadTheFlagListElementByElement(t *testing.T) {
	for _, tt := range []struct {
		name  string
		flags string
		want  bool
	}{
		{name: "the flag itself", flags: "attached,focused,ignore-size,UTF-8", want: true},
		{name: "last in the list", flags: "attached,ignore-size", want: true},
		{name: "absent", flags: "attached,focused,UTF-8", want: false},
		{name: "no flags at all", flags: "", want: false},
		{name: "a longer flag that contains it", flags: "attached,ignore-size-later", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasClientFlag(tt.flags, ignoreSizeFlag); got != tt.want {
				t.Fatalf("hasClientFlag(%q) = %v, want %v", tt.flags, got, tt.want)
			}
		})
	}
}

func TestSizingClients_SkipTheFlaggedAndTheTtyless(t *testing.T) {
	output := strings.Join([]string{
		"/dev/pts/3\tattached,focused,UTF-8",
		"/dev/pts/4\tattached,ignore-size,UTF-8",
		"\tattached,UTF-8", // a control-mode client, which refresh-client cannot address
		"/dev/pts/5\tattached",
	}, "\n") + "\n"

	got := sizingClientTTYsFrom(output)

	if want := []string{"/dev/pts/3", "/dev/pts/5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sizing clients = %v, want %v", got, want)
	}
}
