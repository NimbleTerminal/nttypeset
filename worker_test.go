package nttypeset

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	if len(os.Args) == 3 && os.Args[1] == ntmediabox.StageArg {
		_ = ntmediabox.RunStage(os.Args[1:])
		os.Exit(9)
	}
	if len(os.Args) >= 2 && os.Args[1] == "--nttypeset-test-worker" {
		conn := ntmediabox.WorkerConn()
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
	defer func() { _ = c1.Close() }()
	defer func() { _ = c2.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- ServeWorker(c2, workerTestFontSource{}, Limits{})
	}()

	client := ntmediabox.NewRenderClient(c1)
	defer func() {
		_ = client.Close()
		if err := <-done; err != nil {
			t.Errorf("ServeWorker: %v", err)
		}
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
	if sup := ntmediabox.Probe(); !sup.Available {
		t.Skipf("OS confinement unavailable: %s", sup.Detail)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if runtime.GOOS == "linux" {
		// The Linux stage uses AllThreadsSyscall, which rejects cgo-linked
		// binaries. In particular the race-enabled test executable cannot be
		// its own stage. Build a pure-Go helper for both staging and rendering;
		// race runtimes also need more address space than the worker policy.
		exe = filepath.Join(t.TempDir(), "nttypeset-test-worker")
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "test", "-mod=readonly", "-c", "-o", exe, ".")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=", "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build pure-Go confinement helper: %v\n%s", err, out)
		}
	}

	res, err := RasterWorker(context.Background(), WorkerRequest{
		Text:     "func sandboxed() int { return 42 }",
		Language: "go",
		Dark:     true,
		View:     View{Width: 320, Height: 80, Scale: 1},
	}, WorkerConfig{
		WorkerPath: exe,
		WorkerArgs: []string{"--nttypeset-test-worker"},
		StagePath:  exe,
		Timeout:    5 * time.Second,
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

func TestRasterWorkerPreflight(t *testing.T) {
	for name, view := range map[string]View{
		"zero":            {},
		"infinite":        {Width: 1, Height: 1, Scale: math.Inf(1)},
		"nan":             {Width: 1, Height: 1, Scale: math.NaN()},
		"partial pixels":  {Width: 1, Height: 1, Pixels: image.Pt(1, 0)},
		"side":            {Width: ntmediabox.MaxRenderSide + 1, Height: 1},
		"pixels":          {Width: 8192, Height: 8192},
		"overflow":        {Width: 1, Height: 1, Pixels: image.Pt(math.MaxInt, math.MaxInt)},
		"scaled overflow": {Width: math.MaxInt, Height: 1, Scale: math.MaxFloat64},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := RasterWorker(context.Background(), WorkerRequest{View: view}, WorkerConfig{WorkerPath: "/nonexistent-worker"})
			if err == nil || !strings.HasPrefix(err.Error(), "nttypeset: view") && !errors.Is(err, ErrLimit) {
				t.Fatalf("expected preflight rejection, got %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RasterWorker(ctx, WorkerRequest{View: View{Width: 1, Height: 1}}, WorkerConfig{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request = %v", err)
	}
}

// A valid wire response is still an untrusted application response.
func TestRasterWorkerRejectsHostileMetadata(t *testing.T) {
	valid := workerRespMeta{Extent: 50, ExtentX: 50, Done: true}
	encode := func(m workerRespMeta) []byte {
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	badLink := valid
	badLink.Links = []Link{{Rect: image.Rect(0, 0, 2, 1)}}
	badMark := valid
	badMark.Links = []Link{{Rect: image.Rect(0, 0, 1, 1), Runes: [2]int{2, 1}}}
	for name, meta := range map[string][]byte{
		"missing": nil, "null": []byte("null"), "incomplete": []byte("{}"),
		"malformed": []byte("{"), "negative": []byte(`{"extent":-1,"extent_x":0,"done":false}`),
		"outside viewport": encode(badLink), "invalid mark": encode(badMark),
	} {
		t.Run(name, func(t *testing.T) {
			a, b := net.Pipe()
			done := make(chan error, 1)
			go func() {
				defer func() { _ = b.Close() }()
				done <- ntmediabox.ServeRender(b, ntmediabox.RenderFunc(func(ntmediabox.RenderRequest) (ntmediabox.RenderResponse, error) {
					return ntmediabox.RenderResponse{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Meta: meta}, nil
				}))
			}()
			killed := false
			client := ntmediabox.NewRenderClientWithKill(a, func() { killed = true })
			defer func() {
				_ = client.Close()
				if err := <-done; err != nil {
					t.Error(err)
				}
			}()
			if _, err := RasterWorker(context.Background(), WorkerRequest{View: View{Width: 1, Height: 1}}, WorkerConfig{Client: client}); err == nil {
				t.Fatal("accepted hostile metadata")
			}
			if !killed {
				t.Fatal("invalid metadata did not close the client and invoke its kill hook")
			}
		})
	}
}

func TestRasterWorkerTimeoutKillsClient(t *testing.T) {
	a, b := net.Pipe()
	killed := false
	client := ntmediabox.NewRenderClientWithKill(a, func() { killed = true; _ = b.Close() })
	defer func() { _ = client.Close() }()
	// The peer never reads or replies: cancellation must interrupt the write.
	_, err := RasterWorker(context.Background(), WorkerRequest{View: View{Width: 1, Height: 1}}, WorkerConfig{Client: client, Timeout: 20 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) || !killed {
		t.Fatalf("timeout = %v, kill hook called = %v", err, killed)
	}
}

func TestServeWorkerRejectsEnvelopeBeforeBuilder(t *testing.T) {
	a, b := net.Pipe()
	done := make(chan error, 1)
	called := false
	go func() {
		defer func() { _ = b.Close() }()
		done <- ServeWorkerWithBuilder(b, workerTestFontSource{}, Limits{Pixels: 4}, func(context.Context, []byte, string, Palette) (*Document, error) {
			called = true
			return &Document{}, nil
		})
	}()
	client := ntmediabox.NewRenderClient(a)
	defer func() {
		_ = client.Close()
		if err := <-done; err != nil {
			t.Error(err)
		}
		if called {
			t.Error("builder called for rejected view")
		}
	}()
	for _, req := range []ntmediabox.RenderRequest{
		{Width: 1, Height: 1, MaxPixels: 4, Meta: []byte(`{"view":{"Width":3,"Height":3}}`)},
		{Width: 2, Height: 2, MaxPixels: 4, Meta: []byte(`{"view":{"Width":1,"Height":1}}`)},
		{Width: 2, Height: 2, MaxPixels: 1, Meta: []byte(`{"view":{"Width":2,"Height":2}}`)},
	} {
		req.Kind = "typeset"
		if _, err := client.Do(context.Background(), req); err == nil {
			t.Fatal("worker accepted invalid envelope")
		}
	}
}
