package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"dagger.io/dagger"
	"github.com/magefile/mage/sh"
)

var Default = Build

// Build builds the gograz-meetup api proxy server
func Build() error {
	return sh.RunWith(
		globalEnv(),
		"go",
		"build",
		"-trimpath",
		"-ldflags", "-s -w",
		"-o", "bin/gograz-meetup",
	)
}

// Clean cleans the project from previously built binary
func Clean() error {
	return sh.Rm("bin")
}

// Ci runs the CI pipeline using Dagger
func Ci(ctx context.Context) error {
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		return err
	}
	defer client.Close()

	containerOpts := dagger.ContainerOpts{
		Platform: "linux/amd64",
	}

	rootDir := client.Host().Directory(".")
	goModulesCache := client.CacheVolume("gomodcache")

	goContainer := client.Container(containerOpts).
		From("golang:1.20.2").
		WithMountedCache("/go/pkg/mod", goModulesCache).
		WithMountedDirectory("/src", rootDir).
		WithWorkdir("/src")

	for key, value := range globalEnv() {
		goContainer = goContainer.WithEnvVariable(key, value)
	}

	log.Println("Run tests...")
	goContainer = goContainer.WithExec([]string{"go", "test", "./...", "-v"})
	if _, err := goContainer.ExitCode(ctx); err != nil {
		return err
	}

	log.Println("Build binary...")
	goContainer = goContainer.WithExec([]string{"go", "build",
		"-trimpath",
		"-ldflags", "-s -w",
	})
	if _, err := goContainer.ExitCode(ctx); err != nil {
		return err
	}

	// Do a quick alive check on the generated binary
	backendContainer := goContainer.WithExposedPort(8080).WithExec([]string{"./gograz-meetup", "--addr", "0.0.0.0:8080"})

	code, err := goContainer.
		WithServiceBinding("backend", backendContainer).
		WithExec([]string{"curl", "--fail", "http://backend:8080/alive"}).
		ExitCode(ctx)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("test failed with status code %d", code)
	}

	return nil
}

func globalEnv() map[string]string {
	return map[string]string{
		"CGO_ENABLED": "0",
	}
}
