package argusdev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type contractStage struct{ root, dependencies string }

func (s contractStage) close() error {
	parent, err := filepath.EvalSymlinks(filepath.Dir(s.root))
	if err != nil {
		return err
	}
	expected, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return err
	}
	if !sameContractPath(parent, expected) || !strings.HasPrefix(filepath.Base(s.root), "argus-contract-check-") {
		return errors.New("contract staging cleanup escaped its owned temporary directory")
	}
	link := filepath.Join(s.root, "node_modules")
	if _, err = os.Lstat(link); err == nil {
		target, e := os.Readlink(link)
		if e != nil {
			return fmt.Errorf("contract dependency alias is no longer a link: %w", e)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(s.root, target)
		}
		resolved, e := filepath.EvalSymlinks(target)
		if e != nil {
			return e
		}
		if !sameContractPath(resolved, s.dependencies) {
			return errors.New("contract dependency alias ownership changed")
		}
		if err = os.Remove(link); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll(s.root)
}

func sameContractPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func (a *App) stageContracts(ctx context.Context) (*App, func() error, error) {
	stageRoot, err := os.MkdirTemp("", "argus-contract-check-")
	if err != nil {
		return nil, nil, err
	}
	stage := contractStage{root: stageRoot}
	fail := func(err error) (*App, func() error, error) { return nil, nil, errors.Join(err, stage.close()) }
	paths, err := a.runner.Output(ctx, nil, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return fail(err)
	}
	copyPath := func(relative string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		relative = filepath.FromSlash(relative)
		if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(filepath.ToSlash(relative), "../") {
			return errors.New("contract input path escaped the workspace")
		}
		normalized := filepath.ToSlash(relative)
		if strings.HasPrefix(normalized, "artifacts/") || strings.HasPrefix(normalized, "build/") || strings.HasPrefix(normalized, "node_modules/") || strings.Contains(normalized, "/node_modules/") {
			return nil
		}
		source := filepath.Join(a.root, relative)
		info, err := os.Lstat(source)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("contract input is not a regular file: %s", normalized)
		}
		destination := filepath.Join(stageRoot, relative)
		if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return err
		}
		input, err := os.Open(source)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, err = io.Copy(output, input)
		return errors.Join(err, output.Close())
	}
	for _, relative := range strings.Split(strings.TrimRight(paths, "\x00"), "\x00") {
		if relative != "" {
			if err = copyPath(relative); err != nil {
				return fail(err)
			}
		}
	}
	// Ignored generated inputs are still needed to compile registry generators,
	// including protobuf types which are generated after the Go registries.
	for _, relative := range []string{"internal/gen", "api/openapi/generated", "web/packages/api-client/src/generated"} {
		directory := filepath.Join(a.root, filepath.FromSlash(relative))
		err = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
			if os.IsNotExist(walkErr) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			path, err := filepath.Rel(a.root, path)
			if err != nil {
				return err
			}
			return copyPath(path)
		})
		if err != nil {
			return fail(err)
		}
	}
	dependencyPath := filepath.Join(a.root, "node_modules")
	if info, e := os.Stat(dependencyPath); e == nil && info.IsDir() {
		stage.dependencies, err = filepath.EvalSymlinks(dependencyPath)
		if err != nil {
			return fail(err)
		}
		link := filepath.Join(stageRoot, "node_modules")
		if runtime.GOOS == "windows" {
			err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "cmd.exe", "/d", "/c", "mklink", "/J", link, stage.dependencies)
		} else {
			err = os.Symlink(stage.dependencies, link)
		}
		if err != nil {
			return fail(err)
		}
	} else if e != nil && !os.IsNotExist(e) {
		return fail(e)
	}
	isolated := *a
	isolated.root = stageRoot
	isolated.runner = Runner{Dir: stageRoot, Stdout: a.runner.Stdout, Stderr: a.runner.Stderr}
	return &isolated, stage.close, nil
}
