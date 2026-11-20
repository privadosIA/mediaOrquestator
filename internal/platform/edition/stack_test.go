package edition

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These tests look at the files of the Compose stack (compose.yaml, the Dockerfile, .env.example, deploy/compose/, scripts/compose-init.sh and
// docs/COMPOSE.md). They run in both editions: what they check is the text the PUBLIC tree gets, so the private blocks are cut first, exactly as
// scripts/sync-public.sh cuts them.

// stackFiles lists the files of the stack, relative to the root of the tree.
func stackFiles(t *testing.T, root string) []string {
	t.Helper()
	files := []string{"compose.yaml", "Dockerfile", ".dockerignore", ".env.example", "scripts/compose-init.sh", "docs/COMPOSE.md"}
	entries, err := filepath.Glob(filepath.Join(root, "deploy", "compose", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if fi, err := os.Stat(e); err == nil && !fi.IsDir() {
			rel, _ := filepath.Rel(root, e)
			files = append(files, filepath.ToSlash(rel))
		}
	}
	return files
}

var (
	hashStart   = regexp.MustCompile(`^\s*# private:start\s*$`)
	hashEnd     = regexp.MustCompile(`^\s*# private:end\s*$`)
	hashLine    = regexp.MustCompile(`# \[private-line\]\s*$`)
	htmlStart   = regexp.MustCompile(`^\s*<!-- private:start -->\s*$`)
	htmlEnd     = regexp.MustCompile(`^\s*<!-- private:end -->\s*$`)
	htmlSpan    = regexp.MustCompile(`<!--\[private\]-->.*?<!--\[/private\]-->`)
	htmlPrivate = regexp.MustCompile(`<!--\[private-line\]-->`)
)

// publicText is the file as the public tree gets it: the private blocks and lines cut. A block that does not close is an error.
func publicText(t *testing.T, rel, text string) string {
	t.Helper()
	markdown := strings.HasSuffix(rel, ".md")
	start, end := hashStart, hashEnd
	if markdown {
		start, end = htmlStart, htmlEnd
	}
	var out []string
	skip := false
	for _, l := range strings.Split(text, "\n") {
		switch {
		case start.MatchString(l):
			if skip {
				t.Errorf("%s: nested private:start", rel)
			}
			skip = true
		case end.MatchString(l):
			if !skip {
				t.Errorf("%s: private:end without start", rel)
			}
			skip = false
		case skip:
		case !markdown && hashLine.MatchString(l):
		case markdown && htmlPrivate.MatchString(l):
		default:
			if markdown {
				l = htmlSpan.ReplaceAllString(l, "")
			}
			out = append(out, l)
		}
	}
	if skip {
		t.Errorf("%s: private:start without end", rel)
	}
	return strings.Join(out, "\n")
}

// ownerTraces is what a trace of the owner's machines looks like: a home or a backup path, a private address.
var ownerTraces = regexp.MustCompile(`/home/[a-z]|/mnt/|/Users/|Proyectos|\b192\.168\.\d+\.\d+\b|\b10\.\d+\.\d+\.\d+\b|\b172\.(?:1[6-9]|2\d|3[01])\.\d+\.\d+\b`)

// privateOnly is what exists only in the private edition: the compression service, the source of the .NET ports and their packages.
var privateOnly = regexp.MustCompile(`(?i)ctxpress|\bdotnet/|dotnet-sdk|aspnetcore|get-services\.sh`)

func TestStackFilesSayNothingPrivateInThePublicText(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range stackFiles(t, root) {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			if os.IsNotExist(err) && rel != "compose.yaml" {
				continue
			}
			t.Fatalf("%s: %v", rel, err)
		}
		text := publicText(t, rel, string(b))
		for _, re := range []*regexp.Regexp{ownerTraces, privateOnly, modelWords, showAllOption} {
			if m := re.FindString(text); m != "" {
				t.Errorf("%s says %q in the text of the public tree", rel, strings.TrimSpace(m))
			}
		}
	}
}

// The header of the Compose file, the README and the document of the stack say what the stack leaves out.
func TestStackSaysWhatItLeavesOut(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{"compose.yaml", "README.md", "docs/COMPOSE.md"} {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		text := strings.ToLower(publicText(t, rel, string(b)))
		if !strings.Contains(text, "dashboards-lab") {
			t.Errorf("%s does not say that the dashboard is a separate project", rel)
		}
		if !strings.Contains(text, ".net services") {
			t.Errorf("%s does not say what the stack does about the .NET services", rel)
		}
		if !strings.Contains(text, "unavailable") {
			t.Errorf("%s does not say that the tasks that need them are reported unavailable", rel)
		}
	}
}

// Every variable of .env.example is named in docs/COMPOSE.md.
func TestStackDocumentsEveryVariable(t *testing.T) {
	root := repoRoot(t)
	ex, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(filepath.Join(root, "docs", "COMPOSE.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(publicText(t, ".env.example", string(ex)), "\n") {
		l = strings.TrimPrefix(strings.TrimSpace(l), "#")
		name, _, ok := strings.Cut(l, "=")
		if !ok || name == "" || strings.ContainsAny(name, " \t") || name != strings.ToUpper(name) {
			continue
		}
		if !strings.Contains(string(doc), "`"+name+"`") {
			t.Errorf("docs/COMPOSE.md does not name %s", name)
		}
	}
}

// fakeDocker writes a `docker` that answers like the real one: whether it has the NVIDIA runtime is what changes.
func fakeDocker(t *testing.T, dir string, runtimes string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  --version) echo 'Docker version 29.9.3, build test' ;;\n" +
		"  info) echo 'Runtimes: " + runtimes + "' ;;\n" +
		"  compose) if [ \"$2\" = version ] && [ \"$3\" = --short ]; then echo 5.7.0; else echo 'Docker Compose version v5.7.0'; fi ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// toolDir is a folder with only the programs the script needs (never docker), so a PATH made of it finds no Docker.
func toolDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"bash", "sed", "grep", "cut", "tail", "head", "id", "od", "tr", "mkdir", "chmod", "cp", "dirname", "cat", "openssl"} {
		p, err := exec.LookPath(n)
		if err != nil {
			continue
		}
		if err := os.Symlink(p, filepath.Join(dir, n)); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// stackCopy is a throwaway tree with the init script and the example environment.
func stackCopy(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"scripts/compose-init.sh", ".env.example"} {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runInit(t *testing.T, dir, path string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(filepath.Join(dir, "scripts", "compose-init.sh"), args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + path, "HOME=" + dir}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestComposeInit(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	tools := toolDir(t)

	t.Run("no docker", func(t *testing.T) {
		dir := stackCopy(t)
		out, err := runInit(t, dir, tools)
		if err == nil || !strings.Contains(out, "Docker is not installed") {
			t.Fatalf("want a refusal naming Docker, got %v: %s", err, out)
		}
		if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
			t.Error("an .env was made although Docker is missing")
		}
	})

	t.Run("no nvidia runtime", func(t *testing.T) {
		dir := stackCopy(t)
		bin := t.TempDir()
		fakeDocker(t, bin, "runc io.containerd.runc.v2")
		out, err := runInit(t, dir, bin+":"+tools)
		if err == nil || !strings.Contains(out, "NVIDIA runtime") {
			t.Fatalf("want a refusal naming the NVIDIA runtime, got %v: %s", err, out)
		}
		if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
			t.Error("an .env was made although the runtime is missing")
		}
		if out, err := runInit(t, dir, bin+":"+tools, "--no-gpu"); err != nil {
			t.Fatalf("--no-gpu must skip the check: %v: %s", err, out)
		}
	})

	t.Run("everything in place", func(t *testing.T) {
		dir := stackCopy(t)
		bin := t.TempDir()
		fakeDocker(t, bin, "runc nvidia")
		out, err := runInit(t, dir, bin+":"+tools)
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		env, err := os.ReadFile(filepath.Join(dir, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		fi, _ := os.Stat(filepath.Join(dir, ".env"))
		if fi.Mode().Perm() != 0o600 {
			t.Errorf(".env mode is %v, want 600", fi.Mode().Perm())
		}
		for _, l := range strings.Split(string(env), "\n") {
			name, val, ok := strings.Cut(l, "=")
			if ok && (strings.HasSuffix(name, "PASSWORD") || strings.HasSuffix(name, "TOKEN")) && len(val) < 32 {
				t.Errorf("%s was not generated (%q)", name, val)
			}
			if strings.HasPrefix(l, "UID=") && l == "UID=" {
				t.Error("UID is empty")
			}
		}
		for _, d := range []string{"compose-data/projects", "compose-data/install/bin", "compose-data/install/third_party", "compose-data/config"} {
			if fi, err := os.Stat(filepath.Join(dir, d)); err != nil || !fi.IsDir() {
				t.Errorf("the folder %s was not made", d)
			}
		}
		if strings.Contains(out, "compose-data/config/token") {
			t.Error("the token is read from .env now: the script must not point at a file that does not exist")
		}
		// a second run keeps the .env as it is
		if _, err := runInit(t, dir, bin+":"+tools); err != nil {
			t.Fatal(err)
		}
		again, _ := os.ReadFile(filepath.Join(dir, ".env"))
		if string(again) != string(env) {
			t.Error("a second run changed the .env")
		}
	})
}
