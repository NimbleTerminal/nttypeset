package nttypeset

import (
	"context"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"nimbleterminal.dev/ntmediabox"
	"nimbleterminal.dev/nttypeset/internal/testfonts"
)

type workerTestFontSource struct{}

func (workerTestFontSource) Face(family, variant string) ([]byte, error) {
	switch family {
	case "Literata":
		family = testfonts.Body
	case "Iosevka":
		family = testfonts.Code
	}
	return testfonts.Source.Face(family, variant)
}

func init() {
	if len(os.Args) >= 2 && os.Args[1] == "--nttypeset-test-worker" {
		conn := os.NewFile(3, "media")
		if conn == nil {
			os.Exit(2)
		}
		if err := ServeWorker(conn, workerTestFontSource{}, Limits{}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
}

func TestRasterWorkerInMemoryPipe(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	done := make(chan error, 1)
	go func() {
		done <- ServeWorker(c2, workerTestFontSource{}, Limits{})
	}()

	client := ntmediabox.NewRenderClient(c1)
	defer func() {
		_ = client.Close()
		<-done
	}()

	res, err := RasterWorker(context.Background(), WorkerRequest{
		Text:     "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n",
		Language: "go",
		Numbers:  true,
		Complete: true,
		View:     View{Width: 400, Height: 120, Scale: 1},
	}, WorkerConfig{
		Client:  client,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("RasterWorker: %v", err)
	}
	if res.Image == nil {
		t.Fatal("RasterWorker returned nil Image")
	}
	if res.Image.Bounds().Dx() != 400 || res.Image.Bounds().Dy() != 120 {
		t.Fatalf("bounds = %v, want 400x120", res.Image.Bounds())
	}
	if !res.Done || res.Extent <= Margin {
		t.Fatalf("Done=%v Extent=%d, want Done=true and Extent > %d", res.Done, res.Extent, Margin)
	}
}

func TestRasterWorkerSandboxedSubprocess(t *testing.T) {
	if runtime.GOOS == "js" || runtime.GOOS == "windows" {
		t.Skip("subprocess worker test only runs on unix")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	res, err := RasterWorker(context.Background(), WorkerRequest{
		Text:     "func sandboxed() int { return 42 }",
		Language: "go",
		Dark:     true,
		View:     View{Width: 320, Height: 80, Scale: 1},
	}, WorkerConfig{
		WorkerPath:       exe,
		WorkerArgs:       []string{"--nttypeset-test-worker"},
		AllowUnsandboxed: true,
		Timeout:          5 * time.Second,
	})
	if err != nil {
		t.Fatalf("RasterWorker subprocess: %v", err)
	}
	if res.Image == nil || res.Image.Bounds().Dx() != 320 || res.Image.Bounds().Dy() != 80 {
		t.Fatalf("bounds = %v, want 320x80", res.Image.Bounds())
	}
	if !res.Done {
		t.Fatal("expected Done=true")
	}
}
