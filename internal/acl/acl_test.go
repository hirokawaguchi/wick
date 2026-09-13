package acl

import (
	"path/filepath"
	"testing"

	"github.com/hirokawaguchi/wick/internal/testenv"
)

func TestParseMask(t *testing.T) {
	all := ParseMask("oooooooo:oooooooo:oooooooo:oooooooo")
	if all != 0xffffffff {
		t.Fatalf("%08x", all)
	}
	sys := ParseMask("--------:--------:oo------:--------")
	if sys&FlagSys == 0 || sys&FlagCos == 0 || sys&FlagGen != 0 {
		t.Fatalf("sys mask %08x", sys)
	}
	if !Allowed(FlagGen, all) {
		t.Fatal("gen vs all")
	}
	if Allowed(FlagGen, sys) {
		t.Fatal("gen should not have sys commands")
	}
	if !Allowed(FlagSys, sys) {
		t.Fatal("sys should have sys commands")
	}
}

func TestLookupPrefix(t *testing.T) {
	tbl := &Table{Commands: []Command{
		{Name: "who", Allow: 0xffffffff},
		{Name: "handle", Allow: 0xffffffff},
	}}
	c, ok := tbl.LookupCommand("wh")
	if !ok || c.Name != "who" {
		t.Fatalf("%v %v", c, ok)
	}
}

func TestLoadRepo(t *testing.T) {
	tbl, err := Load(filepath.Join(testenv.Root(t), "etc"))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := tbl.LookupCommand("password")
	if !ok {
		t.Fatal("password")
	}
	if Allowed(FlagGst, c.Allow) {
		t.Fatal("guest should not change password")
	}
	if !Allowed(FlagGen, c.Allow) {
		t.Fatal("gen should change password")
	}
	logc, ok := tbl.LookupCommand("log")
	if !ok || Allowed(FlagGen, logc.Allow) {
		t.Fatal("gen should not have log")
	}
	pn, ok := tbl.LookupCommand("postnews")
	if !ok || Allowed(FlagGen, pn.Allow) || !Allowed(FlagSys, pn.Allow) || !Allowed(FlagCos, pn.Allow) {
		t.Fatalf("postnews mask %+v ok=%v", pn, ok)
	}
	rn, ok := tbl.LookupCommand("readnews")
	if !ok || !Allowed(FlagGen, rn.Allow) {
		t.Fatal("gen should readnews")
	}
	w, ok := tbl.LookupCommand("w")
	if !ok || w.Alias != "who" {
		t.Fatalf("alias w: %+v", w)
	}
}

func TestSignupAndProbationMasks(t *testing.T) {
	tbl, err := Load(filepath.Join(testenv.Root(t), "etc"))
	if err != nil {
		t.Fatal(err)
	}
	// signup はゲストだけ（会員は使わない）
	su, ok := tbl.LookupCommand("signup")
	if !ok || !Allowed(FlagGst, su.Allow) || Allowed(FlagGen, su.Allow) {
		t.Fatalf("signup mask %+v ok=%v", su, ok)
	}
	// off / version はゲストも会員も
	for _, name := range []string{"off", "version"} {
		c, ok := tbl.LookupCommand(name)
		if !ok || !Allowed(FlagGst, c.Allow) || !Allowed(FlagGen, c.Allow) {
			t.Fatalf("%s should allow guest+gen: %+v", name, c)
		}
	}
	// open: ゲスト不可、見習い可
	op, _ := tbl.LookupCommand("open")
	if Allowed(FlagGst, op.Allow) {
		t.Fatal("guest should not open")
	}
	if !Allowed(FlagPro, op.Allow) {
		t.Fatal("probation should read notes")
	}
	// postmail: 見習い不可、一般可
	pm, _ := tbl.LookupCommand("postmail")
	if Allowed(FlagPro, pm.Allow) {
		t.Fatal("probation should not postmail")
	}
	if !Allowed(FlagGen, pm.Allow) {
		t.Fatal("gen should postmail")
	}
	// useredit: sys/cos だけ
	ue, ok := tbl.LookupCommand("useredit")
	if !ok || Allowed(FlagGen, ue.Allow) || !Allowed(FlagSys, ue.Allow) || !Allowed(FlagCos, ue.Allow) {
		t.Fatalf("useredit mask %+v ok=%v", ue, ok)
	}
}
