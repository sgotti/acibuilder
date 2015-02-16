package acibuilder

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/appc/spec/aci"
	"github.com/appc/spec/schema"

	"github.com/sgotti/fsdiffer"
)

// DiffACIBuilder is an ACIBuilder that creates an ACI containing only the
// different files (in the "./rootfs/") between a base and a new ACI.
// basePath and path are the paths of the two already extracted ACIs.
// If there are deleted files from the base ACI, the imagemanifest will be
// augmented with a pathWhiteList containing all the ACI's files
type DiffACIBuilder struct {
	basePath string
	path     string
}

func NewDiffACIBuilder(basePath string, path string) *DiffACIBuilder {
	return &DiffACIBuilder{basePath: basePath, path: path}
}

func (b *DiffACIBuilder) Build(im schema.ImageManifest, out io.Writer) error {
	baseRootFS := filepath.Join(b.basePath, "/rootfs")
	rootFS := filepath.Join(b.path, "/rootfs")

	fsd := fsdiffer.NewSimpleFSDiffer(baseRootFS, rootFS)

	changes, err := fsd.Diff()

	if err != nil {
		return err
	}

	// Create a file list with all the Added and Modified files
	files := make(map[string]struct{})
	hasDeleted := false
	for _, c := range changes {
		if c.ChangeType == fsdiffer.Added || c.ChangeType == fsdiffer.Modified {
			files[filepath.Join(rootFS, c.Path)] = struct{}{}
		}
		if hasDeleted == false && c.ChangeType == fsdiffer.Deleted {
			hasDeleted = true
		}
	}

	// Compose pathWhiteList only if there're some deleted files
	pathWhitelist := []string{}
	if hasDeleted {
		err = filepath.Walk(rootFS, func(path string, info os.FileInfo, err error) error {
			relpath, err := filepath.Rel(rootFS, path)
			if err != nil {
				return err
			}
			pathWhitelist = append(pathWhitelist, filepath.Join("/", relpath))
			return nil
		})

		if err != nil {
			return fmt.Errorf("error walking rootfs: %v", err)
		}
	}

	gw := gzip.NewWriter(out)
	tr := tar.NewWriter(gw)
	defer func() {
		tr.Close()
		gw.Close()
	}()

	im.PathWhitelist = pathWhitelist

	aw := aci.NewImageWriter(im, tr)

	err = filepath.Walk(b.path, BuildWalker(b.path, files, aw))
	if err != nil {
		return fmt.Errorf("error walking rootfs: %v", err)
	}

	err = aw.Close()
	if err != nil {
		return fmt.Errorf("unable to close image %s: %v", out, err)
	}

	return nil
}
