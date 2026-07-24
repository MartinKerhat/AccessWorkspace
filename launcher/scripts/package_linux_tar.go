//go:build ignore

// Packages the Linux launcher binary into a tar.gz with a proper 0755 mode
// entry. Windows tar.exe (bsdtar) does not preserve the executable bit when
// creating archives from NTFS, which would leave users with a non-executable
// binary after extraction; writing the archive in Go sets the mode explicitly.
//
// Usage: go run ./scripts/package_linux_tar.go -binary <path> -name <entry-name> -out <path.tar.gz>
package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"log"
	"os"
	"time"
)

func main() {
	binaryPath := flag.String("binary", "", "path to the built linux binary")
	entryName := flag.String("name", "access-workspace-launcher", "file name inside the archive")
	outPath := flag.String("out", "", "output .tar.gz path")
	flag.Parse()
	if *binaryPath == "" || *outPath == "" {
		log.Fatal("both -binary and -out are required")
	}

	content, err := os.ReadFile(*binaryPath)
	if err != nil {
		log.Fatalf("read binary: %v", err)
	}
	outFile, err := os.Create(*outPath)
	if err != nil {
		log.Fatalf("create archive: %v", err)
	}
	defer outFile.Close()

	gzipWriter := gzip.NewWriter(outFile)
	tarWriter := tar.NewWriter(gzipWriter)
	header := &tar.Header{
		Name:    *entryName,
		Mode:    0o755,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		log.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		log.Fatalf("write tar content: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		log.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		log.Fatalf("close gzip: %v", err)
	}
}
