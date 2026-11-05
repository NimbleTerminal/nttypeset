package nttypeset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"time"

	"nimbleterminal.dev/ntmediabox"
)

// DefaultWorkerTimeout is the default per-request timeout for RasterWorker.
const DefaultWorkerTimeout = 15 * time.Second

// WorkerConfig configures sandboxed out-of-process typesetting over ntmediabox.
type WorkerConfig struct {
	// WorkerPath is the path to the helper executable that invokes ServeWorker.
	// Required unless Client is set.
	WorkerPath string
	// WorkerArgs are optional arguments passed to WorkerPath.
	WorkerArgs []string
	// StagePath is the optional Linux stage helper path.
	StagePath string
	// Policy is the extra sandbox policy merged with DefaultLibPaths().
	Policy ntmediabox.Policy
	// AllowUnsandboxed permits running when OS sandbox enforcement is unavailable.
	AllowUnsandboxed bool
	// Timeout bounds the raster request (default DefaultWorkerTimeout).
	Timeout time.Duration
	// Client is an optional pre-connected RenderClient. When non-nil,
	// RasterWorker uses Client directly and does not close it.
	Client *ntmediabox.RenderClient
}

// WorkerRequest describes a single typesetting + rasterization request sent to
// a confined ntmediabox worker.
type WorkerRequest struct {
	Text         string
	Language     string
	Look         string
	MonoOverride string
	Dark         bool
	Numbers      bool
	Chop         bool
	Complete     bool
	View         View
}

// WorkerResult is the bounds-checked result returned by a confined ntmediabox
// typesetting worker.
type WorkerResult struct {
	Image   *image.RGBA
	Extent  int
	ExtentX int
	Done    bool
	Links   []Link
}

type workerReqMeta struct {
	Language     string `json:"language,omitempty"`
	Look         string `json:"look,omitempty"`
	MonoOverride string `json:"mono_override,omitempty"`
	Numbers      bool   `json:"numbers,omitempty"`
	Chop         bool   `json:"chop,omitempty"`
	Complete     bool   `json:"complete,omitempty"`
	View         View   `json:"view"`
}

type workerRespMeta struct {
	Extent  int    `json:"extent"`
	ExtentX int    `json:"extent_x"`
	Done    bool   `json:"done"`
	Links   []Link `json:"links,omitempty"`
}

// DocumentBuilder parses raw input bytes into a Document inside a worker.
type DocumentBuilder func(ctx context.Context, data []byte, language string, pal Palette) (*Document, error)

// RasterWorker lays out and rasterizes req inside a confined ntmediabox worker,
// returning a bounds-checked *image.RGBA and layout metrics.
func RasterWorker(ctx context.Context, req WorkerRequest, cfg WorkerConfig) (WorkerResult, error) {
	v := req.View
	if v.Scale == 0 {
		v.Scale = 1
	}
	if v.Width < 1 || v.Height < 1 || !(v.Scale > 0) {
		return WorkerResult{}, errors.New("nttypeset: view must have a positive width, height and scale")
	}
	size := v.Size()
	if size.X < 1 || size.Y < 1 {
		return WorkerResult{}, errors.New("nttypeset: view has no pixels")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultWorkerTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := cfg.Client
	if client == nil {
		if cfg.WorkerPath == "" {
			return WorkerResult{}, errors.New("nttypeset: WorkerPath or Client is required")
		}
		policy := cfg.Policy
		if len(policy.ExecPaths) == 0 {
			policy.ExecPaths = []string{cfg.WorkerPath}
		}
		if len(policy.ReadPaths) == 0 {
			policy.ReadPaths = ntmediabox.DefaultLibPaths()
		}
		if cfg.AllowUnsandboxed {
			policy.AllowUnsandboxed = true
		}
		var err error
		client, _, err = ntmediabox.StartRenderHelper(ntmediabox.HelperConfig{
			WorkerPath: cfg.WorkerPath,
			WorkerArgs: cfg.WorkerArgs,
			StagePath:  cfg.StagePath,
			Policy:     policy,
		})
		if err != nil {
			return WorkerResult{}, fmt.Errorf("nttypeset: start worker: %w", err)
		}
		defer func() { _ = client.Close() }()
	}

	metaBytes, err := json.Marshal(workerReqMeta{
		Language:     req.Language,
		Look:         req.Look,
		MonoOverride: req.MonoOverride,
		Numbers:      req.Numbers,
		Chop:         req.Chop,
		Complete:     req.Complete,
		View:         v,
	})
	if err != nil {
		return WorkerResult{}, fmt.Errorf("nttypeset: marshal worker request: %w", err)
	}

	var flags uint32
	if req.Dark {
		flags |= ntmediabox.RenderFlagDark
	}

	resp, err := client.Do(reqCtx, ntmediabox.RenderRequest{
		Kind:      "typeset",
		Width:     size.X,
		Height:    size.Y,
		MaxPixels: size.X * size.Y,
		Flags:     flags,
		Meta:      metaBytes,
		Data:      []byte(req.Text),
	})
	if err != nil {
		return WorkerResult{}, err
	}
	if resp.Image == nil {
		return WorkerResult{}, errors.New("nttypeset: worker returned nil image")
	}
	var outMeta workerRespMeta
	if len(resp.Meta) > 0 {
		if err := json.Unmarshal(resp.Meta, &outMeta); err != nil {
			return WorkerResult{}, fmt.Errorf("nttypeset: decode worker metadata: %w", err)
		}
	}
	return WorkerResult{
		Image:   resp.Image,
		Extent:  outMeta.Extent,
		ExtentX: outMeta.ExtentX,
		Done:    outMeta.Done,
		Links:   outMeta.Links,
	}, nil
}

// ServeWorker runs the worker-side typesetting loop over rwc (typically
// os.NewFile(3, "media")) using Lines as the document builder.
func ServeWorker(rwc io.ReadWriteCloser, fonts FaceSource, limits Limits) error {
	return ServeWorkerWithBuilder(rwc, fonts, limits, func(ctx context.Context, data []byte, language string, pal Palette) (*Document, error) {
		return Lines(ctx, string(data), language, pal.Syntax)
	})
}

// ServeWorkerWithBuilder runs the worker-side typesetting loop over rwc using a
// custom DocumentBuilder (such as goldmark.Build).
func ServeWorkerWithBuilder(rwc io.ReadWriteCloser, fonts FaceSource, limits Limits, build DocumentBuilder) error {
	if fonts == nil {
		return errors.New("nttypeset: fonts FaceSource is required")
	}
	if build == nil {
		return errors.New("nttypeset: DocumentBuilder is required")
	}
	eng, err := NewEngine(WithFonts(fonts), WithLimits(limits))
	if err != nil {
		return err
	}
	limits = limits.withDefaults()

	return ntmediabox.ServeRender(rwc, ntmediabox.RenderFunc(func(req ntmediabox.RenderRequest) (ntmediabox.RenderResponse, error) {
		if req.Kind != "typeset" {
			return ntmediabox.RenderResponse{}, fmt.Errorf("nttypeset: unsupported request kind %q", req.Kind)
		}
		var meta workerReqMeta
		if len(req.Meta) > 0 {
			if err := json.Unmarshal(req.Meta, &meta); err != nil {
				return ntmediabox.RenderResponse{}, fmt.Errorf("nttypeset: invalid worker metadata: %w", err)
			}
		}
		if _, err := eng.SetFonts(meta.Look, meta.MonoOverride); err != nil {
			return ntmediabox.RenderResponse{}, err
		}
		pal := LightPalette()
		if req.Flags&ntmediabox.RenderFlagDark != 0 {
			pal = DarkPalette()
		}

		ctx, cancel := context.WithTimeout(context.Background(), limits.Deadline)
		defer cancel()

		doc, err := build(ctx, req.Data, meta.Language, pal)
		if err != nil {
			return ntmediabox.RenderResponse{}, err
		}
		v := meta.View
		if v.Scale == 0 {
			v.Scale = 1
		}
		lay, err := eng.Layout(doc, v.Width, LayoutOptions{
			Numbers: meta.Numbers,
			Chop:    meta.Chop,
		})
		if err != nil {
			return ntmediabox.RenderResponse{}, err
		}
		if meta.Complete {
			if err := lay.Complete(ctx); err != nil {
				return ntmediabox.RenderResponse{}, err
			}
		}
		img, err := lay.Raster(ctx, v, RasterOptions{Palette: pal})
		if err != nil {
			return ntmediabox.RenderResponse{}, err
		}
		respMeta, err := json.Marshal(workerRespMeta{
			Extent:  lay.Extent(),
			ExtentX: lay.ExtentX(),
			Done:    lay.Done(),
			Links:   lay.Links(v),
		})
		if err != nil {
			return ntmediabox.RenderResponse{}, err
		}
		b := img.Bounds()
		return ntmediabox.RenderResponse{
			Width:  float64(b.Dx()),
			Height: float64(b.Dy()),
			Image:  img,
			Meta:   respMeta,
		}, nil
	}))
}
