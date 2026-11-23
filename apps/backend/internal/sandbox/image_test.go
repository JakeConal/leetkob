package sandbox

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// TestImageMetadataIsUnprivilegedAndSecretFree verifies provenance facts
// that the controller relies on beyond its explicit flags: the image itself
// declares the unprivileged user, exposes no volumes or ports, and carries
// no credential-shaped environment. It complements the runtime probes.
func TestImageMetadataIsUnprivilegedAndSecretFree(t *testing.T) {
	requireDocker(t)

	out, err := exec.Command("docker", "image", "inspect", "--format", "{{json .Config}}", sandboxImage).Output()
	if err != nil {
		t.Fatalf("inspect image config: %v", err)
	}
	var config struct {
		User       string              `json:"User"`
		Env        []string            `json:"Env"`
		Volumes    map[string]struct{} `json:"Volumes"`
		Exposed    map[string]struct{} `json:"ExposedPorts"`
		Entrypoint []string            `json:"Entrypoint"`
		Labels     map[string]string   `json:"Labels"`
	}
	if err := json.Unmarshal(out, &config); err != nil {
		t.Fatalf("decode image config: %v", err)
	}
	if config.User != sandboxUser {
		t.Fatalf("image user = %q, want %q", config.User, sandboxUser)
	}

	// Provenance labels are checked so a silent base/toolchain change
	// fails the suite even if the tag still resolves.
	const (
		wantBaseDigest = "sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2"
		wantGxxPackage = "4:14.2.0-1"
		wantGxxVersion = "14.2.0-19"
	)
	provenance := map[string]string{
		"org.opencontainers.image.base.digest": wantBaseDigest,
		"org.leetkob.toolchain.gxx.package":    wantGxxPackage,
		"org.leetkob.toolchain.gxx.version":    wantGxxVersion,
		"org.opencontainers.image.base.name":   "docker.io/library/debian:trixie-slim",
	}
	for label, want := range provenance {
		if got := config.Labels[label]; got != want {
			t.Fatalf("image label %s = %q, want %q", label, got, want)
		}
	}
	if len(config.Volumes) != 0 {
		t.Fatalf("image declares volumes: %v", config.Volumes)
	}
	if len(config.Exposed) != 0 {
		t.Fatalf("image exposes ports: %v", config.Exposed)
	}
	if len(config.Entrypoint) != 0 {
		t.Fatalf("image has an entrypoint; the controller must supply the command: %v", config.Entrypoint)
	}
	for _, env := range config.Env {
		lower := strings.ToLower(env)
		for _, suspicious := range []string{"key=", "secret", "token", "password", "credential"} {
			if strings.Contains(lower, suspicious) {
				t.Fatalf("image environment looks credential-shaped: %q", env)
			}
		}
	}
}
