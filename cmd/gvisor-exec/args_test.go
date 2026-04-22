package main

import (
	"flag"
	"os"
	"reflect"
	"testing"

	gve "github.com/mtclinton/gvisor-exec"
)

func TestParseArgs_WithDashDash(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var plat string
	fs.StringVar(&plat, "platform", "systrap", "")

	pos, err := parseArgs(fs, []string{"-platform", "ptrace", "--", "uname", "-a"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if plat != "ptrace" {
		t.Errorf("platform = %q, want ptrace", plat)
	}
	want := []string{"uname", "-a"}
	if !reflect.DeepEqual(pos, want) {
		t.Errorf("positional = %v, want %v", pos, want)
	}
}

func TestParseArgs_NoDashDash(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var plat string
	fs.StringVar(&plat, "platform", "systrap", "")

	pos, err := parseArgs(fs, []string{"-platform", "ptrace", "echo", "hi"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if plat != "ptrace" {
		t.Errorf("platform = %q, want ptrace", plat)
	}
	want := []string{"echo", "hi"}
	if !reflect.DeepEqual(pos, want) {
		t.Errorf("positional = %v, want %v", pos, want)
	}
}

func TestParseArgs_OnlyFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var plat string
	fs.StringVar(&plat, "platform", "systrap", "")

	pos, err := parseArgs(fs, []string{"-platform", "kvm"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if plat != "kvm" {
		t.Errorf("platform = %q, want kvm", plat)
	}
	if len(pos) != 0 {
		t.Errorf("positional = %v, want empty", pos)
	}
}

func TestParseArgs_DashDashAtStart(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	pos, err := parseArgs(fs, []string{"--", "sh", "-c", "echo hi"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	want := []string{"sh", "-c", "echo hi"}
	if !reflect.DeepEqual(pos, want) {
		t.Errorf("positional = %v, want %v", pos, want)
	}
}

func TestBuildEnv_DefaultsApplied(t *testing.T) {
	env := buildEnv(nil, nil)

	has := func(prefix string) bool {
		for _, e := range env {
			if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
				return true
			}
		}
		return false
	}
	if !has("PATH=") {
		t.Error("PATH default missing")
	}
	if !has("HOME=") {
		t.Error("HOME default missing")
	}
}

func TestBuildEnv_ExplicitOverrides(t *testing.T) {
	env := buildEnv([]string{"PATH=/custom"}, nil)
	for _, e := range env {
		if e == "PATH=/custom" {
			return
		}
		if e == "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" {
			t.Errorf("explicit PATH was overwritten by default, env=%v", env)
		}
	}
	t.Errorf("PATH=/custom missing, env=%v", env)
}

func TestBuildEnv_Inherit(t *testing.T) {
	t.Setenv("GVE_TEST_VAR", "value123")
	env := buildEnv(nil, []string{"GVE_TEST_VAR", "NOT_SET_VAR_GVE"})

	found := false
	for _, e := range env {
		if e == "GVE_TEST_VAR=value123" {
			found = true
		}
		if e == "NOT_SET_VAR_GVE=" {
			t.Errorf("unset var should not be inherited as empty, env=%v", env)
		}
	}
	if !found {
		t.Errorf("inherited var missing, env=%v", env)
	}
	_ = os.Unsetenv
}

func TestBuildEnv_ExplicitBeatsInherit(t *testing.T) {
	t.Setenv("GVE_TEST2", "from-host")
	env := buildEnv([]string{"GVE_TEST2=from-flag"}, []string{"GVE_TEST2"})
	for _, e := range env {
		if e == "GVE_TEST2=from-host" {
			t.Errorf("inherit overwrote explicit value, env=%v", env)
		}
	}
}

func TestParseMounts_WithoutDest(t *testing.T) {
	got, err := parseMounts([]string{"/usr"})
	if err != nil {
		t.Fatal(err)
	}
	want := []gve.Mount{{Source: "/usr", Destination: "/usr"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseMounts_WithDest(t *testing.T) {
	got, err := parseMounts([]string{"/host/a:/sandbox/a"})
	if err != nil {
		t.Fatal(err)
	}
	want := []gve.Mount{{Source: "/host/a", Destination: "/sandbox/a"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseMounts_Empty(t *testing.T) {
	got, err := parseMounts([]string{})
	if err != nil {
		t.Fatalf("parseMounts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestParseMounts_InvalidEmpty(t *testing.T) {
	_, err := parseMounts([]string{""})
	if err == nil {
		t.Error("expected error for empty mount spec")
	}
}
