//go:build desktop && linux
// +build desktop,linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetectYdotoolInstallPlanPrefersAptGet(t *testing.T) {
	plan := detectYdotoolInstallPlan(fakeLookPath(map[string]string{
		"apt-get": "/usr/bin/apt-get",
		"apt":     "/usr/bin/apt",
	}))
	if plan == nil {
		t.Fatal("expected install plan")
	}
	if plan.Manager != "apt-get" {
		t.Fatalf("expected apt-get plan, got %q", plan.Manager)
	}
	if len(plan.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(plan.Commands))
	}
	if got, want := plan.Commands[0].Name, "apt-get"; got != want {
		t.Fatalf("expected first command %q, got %q", want, got)
	}
	if got, want := plan.Commands[1].Args, []string{"install", "-y", "ydotool"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected install args: got %v want %v", got, want)
	}
}

func TestDetectYdotoolInstallPlanSupportsPacman(t *testing.T) {
	plan := detectYdotoolInstallPlan(fakeLookPath(map[string]string{
		"pacman": "/usr/bin/pacman",
	}))
	if plan == nil {
		t.Fatal("expected install plan")
	}
	if plan.Manager != "pacman" {
		t.Fatalf("expected pacman plan, got %q", plan.Manager)
	}
	if got, want := plan.Commands[0].Args, []string{"-Sy", "--noconfirm", "ydotool"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected pacman args: got %v want %v", got, want)
	}
}

func TestYdotoolSocketCandidatesOrdering(t *testing.T) {
	runtimeDir := "/run/user/1000"
	candidates := ydotoolSocketCandidates("/tmp/custom.sock", runtimeDir, 1000)
	want := []string{
		"/tmp/custom.sock",
		filepath.Join(runtimeDir, "deskgo-ydotool.sock"),
		filepath.Join(runtimeDir, ".ydotool_socket"),
		filepath.Join(runtimeDir, "ydotool.sock"),
		filepath.Join(os.TempDir(), "deskgo-ydotool-1000.sock"),
		"/tmp/.ydotool_socket",
		"/tmp/ydotool.sock",
	}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("unexpected socket candidates: got %v want %v", candidates, want)
	}
}

func TestBuildYdotoolDaemonLaunchFromHelpPrefersSocketOwn(t *testing.T) {
	launch := buildYdotoolDaemonLaunchFromHelp("--socket-own\n--socket-perm\n", "/tmp/deskgo.sock", 1001, 1002)
	wantArgs := []string{"--socket-path", "/tmp/deskgo.sock", "--socket-own", "1001:1002"}
	if !reflect.DeepEqual(launch.Args, wantArgs) {
		t.Fatalf("unexpected args: got %v want %v", launch.Args, wantArgs)
	}
	if !launch.SocketShareable {
		t.Fatal("expected socket to be shareable")
	}
}

func TestBuildYdotoolDaemonLaunchFromHelpFallsBackToSocketPerm(t *testing.T) {
	launch := buildYdotoolDaemonLaunchFromHelp("--socket-perm\n", "/tmp/deskgo.sock", 1001, 1002)
	wantArgs := []string{"--socket-path", "/tmp/deskgo.sock", "--socket-perm", "0666"}
	if !reflect.DeepEqual(launch.Args, wantArgs) {
		t.Fatalf("unexpected args: got %v want %v", launch.Args, wantArgs)
	}
	if !launch.SocketShareable {
		t.Fatal("expected socket to be shareable")
	}
}

func TestBuildYdotoolDaemonLaunchFromHelpWithoutShareOptions(t *testing.T) {
	launch := buildYdotoolDaemonLaunchFromHelp("", "/tmp/deskgo.sock", 1001, 1002)
	wantArgs := []string{"--socket-path", "/tmp/deskgo.sock"}
	if !reflect.DeepEqual(launch.Args, wantArgs) {
		t.Fatalf("unexpected args: got %v want %v", launch.Args, wantArgs)
	}
	if launch.SocketShareable {
		t.Fatal("expected socket to require same-user daemon")
	}
}

func fakeLookPath(values map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if value, ok := values[name]; ok {
			return value, nil
		}
		return "", fmt.Errorf("%s not found", name)
	}
}
