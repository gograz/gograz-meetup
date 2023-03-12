package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"cloud.google.com/go/storage"
	"dagger.io/dagger"
	"github.com/magefile/mage/sh"
	"github.com/rs/zerolog"
)

var logger zerolog.Logger

func init() {
	logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()

}

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
	branch := os.Getenv("GITHUB_REF_NAME")

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

	logger.Info().Msg("Run tests...")
	goContainer = goContainer.WithExec([]string{"go", "test", "./...", "-v"})
	if _, err := goContainer.ExitCode(ctx); err != nil {
		return err
	}

	logger.Info().Msg("Build binary...")
	goContainer = goContainer.WithExec([]string{"go", "build",
		"-trimpath",
		"-ldflags", "-s -w",
	})
	if _, err := goContainer.ExitCode(ctx); err != nil {
		return err
	}

	logger.Info().Msg("Running alive-check...")
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

	if branch == "master" {
		// Upload binary to GCS
		logger.Info().Msg("Exporting binary...")
		if _, err := goContainer.File("./gograz-meetup").Export(ctx, "./gograz-meetup"); err != nil {
			return err
		}

		logger.Info().Msg("Uploading file to GCS...")
		gcs, err := storage.NewClient(ctx)
		if err != nil {
			return err
		}
		writer := gcs.Bucket("gograz-meetup-files").Object("latest/gograz-meetup").NewWriter(ctx)
		fp, err := os.Open("./gograz-meetup")
		if err != nil {
			return err
		}
		if _, err := io.Copy(writer, fp); err != nil {
			fp.Close()
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
	}

	return nil
}

func globalEnv() map[string]string {
	return map[string]string{
		"CGO_ENABLED": "0",
	}
}
