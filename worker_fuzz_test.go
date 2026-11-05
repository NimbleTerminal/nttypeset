package nttypeset

import (
	"context"
	"encoding/json"
	"image"
	"math"
	"net"
	"reflect"
	"testing"
	"time"

	"nimbleterminal.dev/ntmediabox"
)

// FuzzWorkerView exercises arbitrary machine-sized coordinates and IEEE-754
// values without allocating a raster. Accepted views must fit the IPC budget,
// and tightening that budget must never change the requested dimensions.
func FuzzWorkerView(f *testing.F) {
	add := func(w, h, x, y, budget int, scale float64) {
		f.Add(w, h, x, y, budget, math.Float64bits(scale))
	}
	add(400, 120, 0, 0, 48000, 1)
	add(1, 1, 0, 0, 1, 0)
	add(1, 1, 0, 0, 1, math.SmallestNonzeroFloat64)
	add(1, 1, 1, 0, 1, 1)
	add(8192, 4096, 0, 0, ntmediabox.MaxRenderPixels, 1)
	add(8192, 8192, 0, 0, ntmediabox.MaxRenderPixels, 1)
	add(math.MaxInt, 1, 0, 0, 1, math.MaxFloat64)
	add(1, 1, math.MaxInt, math.MaxInt, 1, 1)
	add(1, 1, 0, 0, 1, math.Inf(1))
	add(1, 1, 0, 0, 1, math.NaN())
	f.Fuzz(func(t *testing.T, w, h, x, y, budget int, scaleBits uint64) {
		// The helper receives positive, already-clamped engine/wire budgets.
		budget = 1 + int((uint64(budget)-1)%uint64(ntmediabox.MaxRenderPixels))
		view := View{Width: w, Height: h, Scale: math.Float64frombits(scaleBits), Pixels: image.Pt(x, y)}
		normalized, size, err := checkedWorkerView(view, budget)
		if err != nil {
			return
		}
		if normalized.Width <= 0 || normalized.Height <= 0 || normalized.Scale <= 0 || math.IsNaN(normalized.Scale) || math.IsInf(normalized.Scale, 0) {
			t.Fatalf("accepted invalid view: %+v", normalized)
		}
		if size.X < 1 || size.Y < 1 || size.X > ntmediabox.MaxRenderSide || size.Y > ntmediabox.MaxRenderSide || uint64(size.X)*uint64(size.Y) > uint64(budget) {
			t.Fatalf("accepted raster %v outside budget %d", size, budget)
		}
		if normalized.Size() != size {
			t.Fatal("preflight dimensions differ from the raster dimensions")
		}
		_, larger, err := checkedWorkerView(view, ntmediabox.MaxRenderPixels)
		if err != nil || larger != size {
			t.Fatal("raising the pixel budget changed an accepted view")
		}
		pixels := size.X * size.Y // Checked above using uint64.
		if pixels > 1 {
			if _, _, err := checkedWorkerView(view, pixels-1); err == nil {
				t.Fatal("accepted a budget smaller than the requested raster")
			}
		}
	})
}

// FuzzWorkerMetadata mutates both valid application responses and malformed
// JSON. An accepted response must retain its meaning through re-encoding and
// expose only hit areas that can safely be used within the returned raster.
func FuzzWorkerMetadata(f *testing.F) {
	for _, seed := range []string{
		`{"extent":0,"extent_x":0,"done":false}`,
		`{"extent":100,"extent_x":200,"done":true,"links":[{"Rect":{"Min":{"X":0,"Y":0},"Max":{"X":64,"Y":64}},"URL":"https://example.invalid/","Block":0,"Para":0,"Runes":[0,3]}]}`,
		`{"extent":0,"extent_x":0,"done":false,"links":[{"Rect":{"Min":{"X":-1,"Y":0},"Max":{"X":2,"Y":2}},"Runes":[0,1]}]}`,
		`{"extent":-1,"extent_x":0,"done":true}`,
		`{"extent":0,"extent_x":0,"done":null}`,
		`{"extent":0,"extent_x":0,"done":true,"links":[{"Runes":[2,1]}]}`,
		`{"extent":0,"extent_x":0,"done":true,"links":{}}`,
		`{"extent":0,"extent":null,"extent_x":0,"done":false}`,
		`{"extent":9223372036854775808,"extent_x":0,"done":false}`,
		`null`, `{}`, `[1]`, `{`, "", "\xff",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// Keep mutation campaigns cheap; production accepts up to the wire cap.
		if len(data) > 64<<10 {
			t.Skip()
		}
		bounds := image.Rect(0, 0, 64, 64)
		meta, err := decodeWorkerMeta(data, bounds)
		if err != nil {
			return
		}
		if meta.Extent < 0 || meta.ExtentX < 0 || len(meta.Links) > defaultLimits.Operations {
			t.Fatalf("unsafe layout metadata: %+v", meta)
		}
		for _, link := range meta.Links {
			if link.Rect.Empty() || link.Rect.Intersect(bounds) != link.Rect || link.Block < 0 || link.Block >= defaultLimits.Blocks || link.Para < 0 || link.Runes[0] < 0 || link.Runes[0] > link.Runes[1] || link.Runes[1] > defaultLimits.Runes {
				t.Fatalf("unsafe link: %+v", link)
			}
		}
		encoded, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip, err := decodeWorkerMeta(encoded, bounds)
		// Empty arrays and omitted links have the same application meaning.
		if len(meta.Links) == 0 {
			meta.Links = nil
		}
		if err != nil || !reflect.DeepEqual(meta, roundTrip) {
			t.Fatalf("metadata changed on round trip: %v", err)
		}
	})
}

// FuzzWorkerRoundTrip reaches the actual builder, shaping, raster and metadata
// path through RenderClient. It uses a bounded in-memory transport, not OS
// subprocesses, so it does not establish confinement.
func FuzzWorkerRoundTrip(f *testing.F) {
	f.Add("hello", "https://example.invalid/")
	f.Add("", "")
	f.Add("a\n\tb\x00c", "javascript:alert(1)")
	f.Add("日本語 🙂 مرحبا", "\x1b[31m\u202e")
	f.Add("\xff\xfe", "\xff")
	f.Fuzz(func(t *testing.T, text, href string) {
		if len(text) > 512 || len(href) > 512 {
			t.Skip()
		}
		// JSON strings replace invalid UTF-8. Make that normalization explicit
		// before testing that the worker preserves an opaque link destination.
		href = string([]rune(href))
		a, b := net.Pipe()
		done := make(chan error, 1)
		go func() {
			defer func() { _ = b.Close() }()
			done <- ServeWorkerWithBuilder(b, workerTestFontSource{}, Limits{Pixels: 128 * 128}, func(_ context.Context, data []byte, _ string, _ Palette) (*Document, error) {
				return &Document{Blocks: []Block{{Spans: []Span{{Text: string(data), Href: href}}}}}, nil
			})
		}()
		client := ntmediabox.NewRenderClient(a)
		defer func() {
			_ = client.Close()
			if err := <-done; err != nil {
				t.Error(err)
			}
		}()
		result, err := RasterWorker(context.Background(), WorkerRequest{Text: text, View: View{Width: 128, Height: 128}}, WorkerConfig{Client: client, Timeout: 5 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if result.Image.Bounds() != image.Rect(0, 0, 128, 128) {
			t.Fatalf("unexpected raster: %v", result.Image.Bounds())
		}
		for _, link := range result.Links {
			if link.URL != href {
				t.Fatal("worker changed the link destination")
			}
		}
	})
}
