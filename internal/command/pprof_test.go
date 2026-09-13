package command

import (
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/session"
)

func TestRegprofProfileSearch(t *testing.T) {
	ctx, st, e := mailEnv(t)
	alice := e.Sess.User

	e.Sess = session.New("t", strings.NewReader("東京の alice\n好きなボードは junk\n.\n"), new(strings.Builder))
	e.Sess.User = alice
	if err := cmdRegprof(e); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetUser(ctx, "alice")
	if err != nil || !strings.Contains(u.Profile, "東京の alice") {
		t.Fatalf("prof %q err %v", u.Profile, err)
	}

	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader(""), &out)
	e.Sess.User = alice
	e.Args = "alice"
	if err := cmdProfile(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "東京の alice") {
		t.Fatalf("profile %q", out.String())
	}
	if strings.Contains(out.String(), u.RealName) && u.RealName != "" {
		t.Fatalf("private leaked %q", out.String())
	}

	out.Reset()
	e.Sess = session.New("t", strings.NewReader(""), &out)
	e.Sess.User = alice
	e.Args = ""
	if err := cmdReadprof(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "junk") {
		t.Fatalf("readprof %q", out.String())
	}

	out.Reset()
	e.Sess = session.New("t", strings.NewReader(""), &out)
	e.Sess.User = alice
	e.Args = "東京"
	if err := cmdSearchprof(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "alice") {
		t.Fatalf("search %q", out.String())
	}

	out.Reset()
	e.Sess = session.New("t", strings.NewReader(""), &out)
	e.Sess.User = alice
	e.Args = "存在しない語"
	if err := cmdSearchprof(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ヒットなし") {
		t.Fatalf("miss %q", out.String())
	}
}
