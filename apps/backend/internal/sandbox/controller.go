package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Controller talks to the local Docker daemon. The zero value is not ready;
// use New. Docker and Image override the binary and the image tag.
type Controller struct {
	Docker string
	Image  string
}

// New returns a controller for the fixed C++17 image.
func New() *Controller {
	return &Controller{Docker: "docker", Image: ImageRef}
}

func (c *Controller) dockerBin() string {
	name := "docker"
	if c != nil && c.Docker != "" {
		name = c.Docker
	}
	if abs, err := CommandPath(name); err == nil {
		return abs
	}
	return name
}

// CommandPath resolves a tool to an absolute path so execution does not
// search PATH again.
func CommandPath(name string) (string, error) {
	if filepath.IsAbs(name) {
		return name, nil
	}
	found, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(found)
}

func (c *Controller) imageRef() string {
	if c != nil && c.Image != "" {
		return c.Image
	}
	return ImageRef
}

// Build builds ImageRef from dir and qualifies it by running g++ and
// dpkg-query inside a locked container. dir is the directory that contains
// the Dockerfile; it is not mounted into student containers.
func (c *Controller) Build(ctx context.Context, dir string) (ImageProvenance, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ImageProvenance{}, fmt.Errorf("%w: image directory", ErrPolicy)
	}
	if _, _, err := c.exec(ctx, "build", "-t", c.imageRef(), dir); err != nil {
		return ImageProvenance{}, err
	}
	return c.qualify(ctx)
}

func (c *Controller) qualify(ctx context.Context) (ImageProvenance, error) {
	raw, _, err := c.exec(ctx, "image", "inspect", c.imageRef())
	if err != nil {
		return ImageProvenance{}, err
	}
	var images []struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(raw, &images); err != nil || len(images) != 1 || images[0].ID == "" {
		return ImageProvenance{}, fmt.Errorf("sandbox: image inspect: %w", err)
	}
	compiler, _, err := c.run(ctx, runRequest{
		command:     []string{"g++", "--version"},
		wall:        CompileWall,
		outputLimit: 64 << 10,
		profile:     compileProfile(),
	})
	if err != nil {
		return ImageProvenance{}, err
	}
	if compiler.ExitCode != 0 || compiler.WallTimedOut || compiler.OutputLimited {
		return ImageProvenance{}, fmt.Errorf("sandbox: compiler probe exit %d", compiler.ExitCode)
	}
	packages, _, err := c.run(ctx, runRequest{
		command:     []string{"dpkg-query", "-W", "g++", "gcc"},
		wall:        CompileWall,
		outputLimit: 64 << 10,
		profile:     compileProfile(),
	})
	if err != nil {
		return ImageProvenance{}, err
	}
	if packages.ExitCode != 0 || packages.WallTimedOut {
		return ImageProvenance{}, fmt.Errorf("sandbox: package probe exit %d", packages.ExitCode)
	}
	return ImageProvenance{
		Ref:        c.imageRef(),
		ImageID:    images[0].ID,
		Compiler:   string(compiler.Stdout),
		Packages:   string(packages.Stdout),
		Invocation: compiler.Invocation,
	}, nil
}

// Compile compiles source with g++ -std=c++17 -O2 inside a locked container.
// A non-zero compiler status is reported on CompileResult and is not an error.
func (c *Controller) Compile(ctx context.Context, source []byte) (CompileResult, error) {
	return c.compileWith(ctx, source, compileProfile())
}

func (c *Controller) compileWith(ctx context.Context, source []byte, prof profile) (CompileResult, error) {
	if len(source) == 0 || len(source) > MaxSourceBytes {
		return CompileResult{}, fmt.Errorf("%w: source is %d bytes", ErrPolicy, len(source))
	}
	obs, binary, err := c.run(ctx, runRequest{
		files:       map[string][]byte{"main.cpp": source},
		command:     compileCommand,
		wall:        CompileWall,
		outputLimit: DefaultOutputBytes,
		profile:     prof,
		extract:     "/work/prog",
	})
	result := CompileResult{Observation: obs}
	if err != nil {
		return result, err
	}
	result.Binary = binary
	result.OK = obs.ExitCode == 0 && !obs.WallTimedOut && !obs.OutputLimited && len(binary) > 0
	return result, nil
}

// Execute runs a binary produced by Compile. argv is appended after the
// program and is never interpreted by a shell. The returned error is only
// an infrastructure or policy failure. Execute passes no stdin; judging
// passes each test through ExecuteInput.
func (c *Controller) Execute(ctx context.Context, program []byte, policy RunPolicy, argv []string) (Observation, error) {
	prof, err := runProfile(policy)
	if err != nil {
		return Observation{}, err
	}
	return c.executeWith(ctx, program, prof, policy.Wall, policy.OutputLimit, argv, nil)
}

// ExecuteInput runs a binary with stdin set to one test input. A nil stdin
// is the same as Execute. An empty slice still attaches stdin so the
// program observes EOF. The bytes are copied into the container process,
// not mounted from the host and not placed in the environment.
func (c *Controller) ExecuteInput(ctx context.Context, program []byte, policy RunPolicy, argv []string, stdin []byte) (Observation, error) {
	prof, err := runProfile(policy)
	if err != nil {
		return Observation{}, err
	}
	if stdin == nil {
		stdin = []byte{}
	}
	return c.executeWith(ctx, program, prof, policy.Wall, policy.OutputLimit, argv, stdin)
}

func (c *Controller) executeWith(ctx context.Context, program []byte, prof profile, wall time.Duration, outputLimit int, argv []string, stdin []byte) (Observation, error) {
	if len(program) == 0 || len(program) > maxExtract {
		return Observation{}, fmt.Errorf("%w: program is %d bytes", ErrPolicy, len(program))
	}
	command := make([]string, 0, 1+len(argv))
	command = append(command, "/work/prog")
	command = append(command, argv...)
	obs, _, err := c.run(ctx, runRequest{
		files:       map[string][]byte{"prog": program},
		command:     command,
		wall:        wall,
		outputLimit: outputLimit,
		profile:     prof,
		stdin:       stdin,
	})
	return obs, err
}

type runRequest struct {
	files       map[string][]byte
	command     []string
	wall        time.Duration
	outputLimit int
	profile     profile
	extract     string
	stdin       []byte
}

func (c *Controller) run(ctx context.Context, req runRequest) (Observation, []byte, error) {
	if req.wall <= 0 || req.outputLimit < 1 {
		return Observation{}, nil, fmt.Errorf("%w: wall and output limit are required", ErrPolicy)
	}
	name, err := newName()
	if err != nil {
		return Observation{}, nil, err
	}
	if req.stdin != nil {
		req.profile.OpenStdin = true
	}
	args, err := createArgs(name, c.imageRef(), req.profile, req.command)
	if err != nil {
		return Observation{}, nil, err
	}
	if _, _, err := c.exec(ctx, args...); err != nil {
		return Observation{Name: name, Invocation: args}, nil, err
	}
	removed := false
	defer func() {
		if !removed {
			_ = c.remove(name)
		}
	}()
	if err := c.copyFiles(ctx, name, req.files); err != nil {
		return Observation{Name: name, Invocation: args}, nil, err
	}
	stdout, stderr, wallHit, outputHit, attachErr := c.attach(ctx, name, req.wall, req.outputLimit, req.stdin)
	obs := Observation{
		Name:          name,
		Stdout:        stdout,
		Stderr:        stderr,
		OutputLimited: outputHit,
		WallTimedOut:  wallHit,
		Invocation:    args,
	}
	if errors.Is(attachErr, context.Canceled) || errors.Is(attachErr, context.DeadlineExceeded) {
		_ = c.remove(name)
		removed = true
		obs.ContainerRemoved = c.gone(name)
		return obs, nil, attachErr
	}
	cleanCtx, cancel := cleanupContext()
	defer cancel()
	raw, _, inspectErr := c.exec(cleanCtx, "inspect", name)
	if inspectErr != nil {
		_ = c.remove(name)
		removed = true
		return obs, nil, inspectErr
	}
	doc, err := parseInspect(raw)
	if err != nil {
		_ = c.remove(name)
		removed = true
		return obs, nil, err
	}
	fillObservation(&obs, doc)
	var binary []byte
	if req.extract != "" && doc.State.ExitCode == 0 && !wallHit && !outputHit {
		binary, err = c.extract(cleanCtx, name, req.extract)
		if err != nil {
			_ = c.remove(name)
			removed = true
			obs.ContainerRemoved = c.gone(name)
			return obs, nil, err
		}
	}
	if err := c.remove(name); err != nil {
		return obs, binary, err
	}
	removed = true
	obs.ContainerRemoved = c.gone(name)
	return obs, binary, nil
}

func fillObservation(obs *Observation, doc inspectDoc) {
	obs.ExitCode = doc.State.ExitCode
	obs.OOMKilled = doc.State.OOMKilled
	obs.MemoryBytes = doc.HostConfig.Memory
	obs.MemorySwapBytes = doc.HostConfig.MemorySwap
	obs.NetworkMode = doc.HostConfig.NetworkMode
	obs.User = doc.Config.User
	obs.Privileged = doc.HostConfig.Privileged
	obs.Binds = doc.HostConfig.Binds
	obs.CapDrop = doc.HostConfig.CapDrop
	obs.SecurityOpt = doc.HostConfig.SecurityOpt
	obs.Env = doc.Config.Env
	if doc.HostConfig.PidsLimit != nil {
		obs.PidsLimit = *doc.HostConfig.PidsLimit
	}
	for _, ulimit := range doc.HostConfig.Ulimits {
		if ulimit.Name == "cpu" {
			obs.CPUSeconds = ulimit.Soft
		}
	}
	for _, mount := range doc.Mounts {
		obs.MountSources = append(obs.MountSources, mount.Source)
		obs.MountDestinations = append(obs.MountDestinations, mount.Destination)
	}
}

func (c *Controller) attach(ctx context.Context, name string, wall time.Duration, limit int, stdin []byte) (stdout, stderr []byte, wallHit, outputHit bool, err error) {
	startArgs := []string{"start", "-a"}
	if stdin != nil {
		startArgs = append(startArgs, "-i")
	}
	startArgs = append(startArgs, name)
	cmd := exec.Command(c.dockerBin(), startArgs...)
	if stdin != nil {
		// os/exec copies this in the background. The wall kill unblocks a
		// program that never reads it. The bytes are the test input only.
		cmd.Stdin = bytes.NewReader(stdin)
	}
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, false, false, err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, false, false, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, false, false, err
	}
	hit := make(chan struct{}, 1)
	shared := &capped{limit: limit, hit: hit}
	done := make(chan struct{})
	go func() {
		readInto(outPipe, shared, &stdout)
		done <- struct{}{}
	}()
	go func() {
		readInto(errPipe, shared, &stderr)
		done <- struct{}{}
	}()
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	timer := time.NewTimer(wall)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		c.kill(name)
		<-waitCh
		err = ctx.Err()
	case <-hit:
		c.kill(name)
		<-waitCh
		outputHit = true
	case <-timer.C:
		c.kill(name)
		<-waitCh
		wallHit = true
	case <-waitCh:
	}
	<-done
	<-done
	if shared.tripped() {
		outputHit = true
	}
	return stdout, stderr, wallHit, outputHit, err
}

func (c *Controller) copyFiles(ctx context.Context, name string, files map[string][]byte) error {
	if len(files) == 0 {
		return nil
	}
	dir, err := os.MkdirTemp("", "leetkob-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for filename, body := range files {
		if err := safeWorkFile(filename); err != nil {
			return err
		}
		path := filepath.Join(dir, filename)
		mode := os.FileMode(0o644)
		if filename == "prog" {
			mode = 0o755
		}
		if err := os.WriteFile(path, body, mode); err != nil {
			return err
		}
		// docker cp copies bytes into the container filesystem. It is not a
		// bind mount, and this process never executes the staged file.
		if _, _, err := c.exec(ctx, "cp", path, name+":/work/"+filename); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) extract(ctx context.Context, name, containerPath string) ([]byte, error) {
	if containerPath != "/work/prog" {
		return nil, fmt.Errorf("%w: extract path", ErrInsecureInvocation)
	}
	dir, err := os.MkdirTemp("", "leetkob-extract-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	dest := filepath.Join(dir, "prog")
	if _, _, err := c.exec(ctx, "cp", name+":"+containerPath, dest); err != nil {
		return nil, err
	}
	info, err := os.Stat(dest)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxExtract {
		return nil, fmt.Errorf("%w: artifact is %d bytes", ErrPolicy, info.Size())
	}
	// The artifact is untrusted compiler output. Callers may only pass it
	// back through Execute, which copies it into a new container.
	return os.ReadFile(dest)
}

func (c *Controller) kill(name string) {
	ctx, cancel := cleanupContext()
	defer cancel()
	_, _, _ = c.exec(ctx, "kill", name)
}

func (c *Controller) remove(name string) error {
	ctx, cancel := cleanupContext()
	defer cancel()
	_, _, err := c.exec(ctx, "rm", "-f", name)
	return err
}

func (c *Controller) gone(name string) bool {
	for attempt := 0; attempt < 10; attempt++ {
		ctx, cancel := cleanupContext()
		_, _, err := c.exec(ctx, "inspect", name)
		cancel()
		if err != nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func (c *Controller) exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("sandbox: empty docker argv")
	}
	cmd := exec.CommandContext(ctx, c.dockerBin(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		detail := bytes.TrimSpace(stderr.Bytes())
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("docker %s: %w: %s", args[0], err, detail)
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func newName() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "leetkob-sbx-" + hex.EncodeToString(buf[:]), nil
}

func cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 20*time.Second)
}

var fileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func safeWorkFile(name string) error {
	if strings.Contains(name, "..") || !fileNamePattern.MatchString(name) {
		return fmt.Errorf("%w: work file %q", ErrInsecureInvocation, name)
	}
	return nil
}

type inspectDoc struct {
	State struct {
		ExitCode  int    `json:"ExitCode"`
		OOMKilled bool   `json:"OOMKilled"`
		Status    string `json:"Status"`
	} `json:"State"`
	HostConfig struct {
		Memory      int64    `json:"Memory"`
		MemorySwap  int64    `json:"MemorySwap"`
		NetworkMode string   `json:"NetworkMode"`
		Privileged  bool     `json:"Privileged"`
		PidsLimit   *int64   `json:"PidsLimit"`
		Binds       []string `json:"Binds"`
		CapDrop     []string `json:"CapDrop"`
		SecurityOpt []string `json:"SecurityOpt"`
		Ulimits     []struct {
			Name string `json:"Name"`
			Soft int64  `json:"Soft"`
			Hard int64  `json:"Hard"`
		} `json:"Ulimits"`
	} `json:"HostConfig"`
	Config struct {
		User string   `json:"User"`
		Env  []string `json:"Env"`
	} `json:"Config"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

func parseInspect(raw []byte) (inspectDoc, error) {
	var docs []inspectDoc
	if err := json.Unmarshal(raw, &docs); err != nil {
		return inspectDoc{}, fmt.Errorf("sandbox: inspect: %w", err)
	}
	if len(docs) != 1 {
		return inspectDoc{}, fmt.Errorf("sandbox: inspect returned %d documents", len(docs))
	}
	return docs[0], nil
}
