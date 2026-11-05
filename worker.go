package nttypeset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
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
	// Policy controls sandbox permissions. Empty ReadPaths and ExecPaths use
	// DefaultLibPaths and DefaultExecPaths plus WorkerPath, respectively.
	// Nonempty lists replace those defaults; WritePaths is never added here.
	Policy ntmediabox.Policy
	// AllowUnsandboxed permits running when OS sandbox enforcement is unavailable.
	AllowUnsandboxed bool
	// Timeout bounds the RPC (default DefaultWorkerTimeout). Synchronous
	// process startup is not interrupted by this timeout.
	Timeout time.Duration
	// Client is an optional pre-connected RenderClient. When non-nil,
	// RasterWorker leaves Client open on success. Invalid responses and RPC
	// cancellation close it. The caller supplies confinement and a process kill
	// hook; a bare RenderClient provides neither.
	Client *ntmediabox.RenderClient
}

// WorkerRequest describes a single typesetting + rasterization request sent to
// an ntmediabox worker.
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

// WorkerResult is the validated result returned by a typesetting worker.
// Link geometry and positions are checked; URLs remain untrusted input.
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
// It must bound parsing, nesting and external resource loading itself;
// engine limits only apply once a Document has been built.
type DocumentBuilder func(ctx context.Context, data []byte, language string, pal Palette) (*Document, error)

// RasterWorker lays out and rasterizes req inside an ntmediabox worker,
// returning a bounds-checked *image.RGBA and layout metrics.
func RasterWorker(ctx context.Context, req WorkerRequest, cfg WorkerConfig) (WorkerResult, error) {
	v, size, err := checkedWorkerView(req.View, ntmediabox.MaxRenderPixels)
	if err != nil {
		return WorkerResult{}, err
	}
	if len(req.Text) > ntmediabox.MaxRenderDataBytes {
		return WorkerResult{}, &LimitError{What: "typeset worker input exceeds byte limit", Limit: ntmediabox.MaxRenderDataBytes}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return WorkerResult{}, err
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
			policy.ExecPaths = append(ntmediabox.DefaultExecPaths(), cfg.WorkerPath)
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
	if resp.Image == nil || resp.Image.Bounds() != image.Rect(0, 0, size.X, size.Y) {
		_ = client.Close()
		return WorkerResult{}, errors.New("nttypeset: worker returned unexpected raster dimensions")
	}
	outMeta, err := decodeWorkerMeta(resp.Meta, resp.Image.Bounds())
	if err != nil {
		_ = client.Close()
		return WorkerResult{}, err
	}
	return WorkerResult{
		Image:   resp.Image,
		Extent:  outMeta.Extent,
		ExtentX: outMeta.ExtentX,
		Done:    outMeta.Done,
		Links:   outMeta.Links,
	}, nil
}

// checkedWorkerView validates before float-to-int conversion or multiplication.
// Both the host and worker enforce the same raster envelope.
func checkedWorkerView(v View, maxPixels int) (View, image.Point, error) {
	if v.Scale == 0 {
		v.Scale = 1
	}
	if v.Width < 1 || v.Height < 1 || !(v.Scale > 0) || math.IsInf(v.Scale, 0) {
		return v, image.Point{}, errors.New("nttypeset: view must have a positive width, height and finite scale")
	}
	if v.Pixels != (image.Point{}) && (v.Pixels.X < 1 || v.Pixels.Y < 1) {
		return v, image.Point{}, errors.New("nttypeset: view has incomplete pixel dimensions")
	}
	if v.Pixels == (image.Point{}) && (math.Round(float64(v.Width)*v.Scale) > ntmediabox.MaxRenderSide || math.Round(float64(v.Height)*v.Scale) > ntmediabox.MaxRenderSide) {
		return v, image.Point{}, &LimitError{What: "typeset worker raster exceeds side limit", Limit: ntmediabox.MaxRenderSide}
	}
	size := v.Size()
	if size.X > ntmediabox.MaxRenderSide || size.Y > ntmediabox.MaxRenderSide {
		return v, size, &LimitError{What: "typeset worker raster exceeds side limit", Limit: ntmediabox.MaxRenderSide}
	}
	if size.X > maxPixels/size.Y {
		return v, size, &LimitError{What: "typeset worker raster exceeds pixel limit", Limit: int64(maxPixels)}
	}
	return v, size, nil
}

func decodeWorkerMeta(data []byte, bounds image.Rectangle) (workerRespMeta, error) {
	// Pointers distinguish required fields from their valid zero values.
	var wire struct {
		Extent  *int   `json:"extent"`
		ExtentX *int   `json:"extent_x"`
		Done    *bool  `json:"done"`
		Links   []Link `json:"links"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return workerRespMeta{}, fmt.Errorf("nttypeset: decode worker metadata: %w", err)
	}
	if wire.Extent == nil || wire.ExtentX == nil || wire.Done == nil || *wire.Extent < 0 || *wire.ExtentX < 0 || len(wire.Links) > defaultLimits.Operations {
		return workerRespMeta{}, errors.New("nttypeset: invalid worker layout metadata")
	}
	for _, link := range wire.Links {
		if link.Rect.Empty() || !link.Rect.In(bounds) || link.Block < 0 || link.Block >= defaultLimits.Blocks || link.Para < 0 || link.Runes[0] < 0 || link.Runes[1] < link.Runes[0] || link.Runes[1] > defaultLimits.Runes {
			return workerRespMeta{}, errors.New("nttypeset: invalid worker link metadata")
		}
	}
	return workerRespMeta{Extent: *wire.Extent, ExtentX: *wire.ExtentX, Done: *wire.Done, Links: wire.Links}, nil
}

// ServeWorker runs the worker-side typesetting loop over rwc (typically
// ntmediabox.WorkerConn()) using Lines as the document builder.
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
		maxPixels := limits.Pixels
		if req.MaxPixels > 0 {
			maxPixels = min(maxPixels, req.MaxPixels)
		}
		v, size, err := checkedWorkerView(meta.View, maxPixels)
		if err != nil {
			return ntmediabox.RenderResponse{}, err
		}
		if req.Width != size.X || req.Height != size.Y {
			return ntmediabox.RenderResponse{}, errors.New("nttypeset: worker view and raster envelope disagree")
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
